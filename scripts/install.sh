#!/usr/bin/env bash

set -euo pipefail

INSTALL_VERSION="${VOLANT_VERSION:-latest}"
INSTALL_FORCE=no
RUN_SETUP=yes
NONINTERACTIVE=no
KERNEL_URL="${VOLANT_KERNEL_URL:-}"
VMLINUX_URL="${VOLANT_VMLINUX_URL:-}"
WORK_DIR="/var/lib/volant"
KERNEL_DIR="${WORK_DIR}/kernel"
BZIMAGE_PATH="${KERNEL_DIR}/bzImage"
VMLINUX_PATH="${KERNEL_DIR}/vmlinux"

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TMP_DIR=""

log_info() { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
log_warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*" >&2; }
log_error() { printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; }

cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}

trap cleanup EXIT INT TERM

usage() {
  cat <<EOF
volant Installer

Usage: install.sh [options]

Options:
  --version <ver>     Install a specific version (default: latest)
  --force             Reinstall even if volant is already present
  --skip-setup        Skip running 'volar setup' after installation
  --kernel-url <url>  Download kernel bzImage from URL (default attempts repo kernels path)
  --vmlinux-url <url> Download vmlinux kernel from URL (optional)
  --yes               Non-interactive mode (assume yes to prompts)
  --help              Show this message
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        INSTALL_VERSION="$2"; shift 2 ;;
      --force)
        INSTALL_FORCE=yes; shift ;;
      --skip-setup)
        RUN_SETUP=no; shift ;;
      --kernel-url)
        KERNEL_URL="$2"; shift 2 ;;
      --vmlinux-url)
        VMLINUX_URL="$2"; shift 2 ;;
      --yes|-y)
        NONINTERACTIVE=yes; shift ;;
      --help|-h)
        usage; exit 0 ;;
      *)
        log_error "Unknown option: $1"
        usage
        exit 1
        ;;
    esac
  done
}

require_program() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log_error "Required program '$1' is not installed."
    return 1
  fi
}

require_figlet_font() {
  local font="$1"
  if ! figlet -f "$font" "" >/dev/null 2>&1; then
    return 1
  fi
}

render_banner() {
  if ! command -v figlet >/dev/null 2>&1; then
    log_info "volant Installer"
    return
  fi
  local font="terrace"
  if ! require_figlet_font "$font"; then
    font="pagga"
  fi
  if ! require_figlet_font "$font"; then
    log_info "volant Installer"
    return
  fi
  figlet -f "$font" "volant"
}

check_shell() {
  if [[ -z "${BASH_VERSION:-}" ]]; then
    log_error "This installer must be run with bash. Use 'bash install.sh'."
    exit 1
  fi
}

detect_os() {
  local os=""; local pkg=""; local update_cmd=""; local install_cmd=""
  if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    case "$ID" in
      ubuntu|debian)
        os="debian"; pkg="apt"; update_cmd="sudo apt update"; install_cmd="sudo apt install -y" ;;
      fedora)
        os="fedora"; pkg="dnf"; update_cmd="sudo dnf makecache"; install_cmd="sudo dnf install -y" ;;
      centos|rhel)
        os="rhel"; pkg="yum"; update_cmd="sudo yum makecache"; install_cmd="sudo yum install -y" ;;
      arch)
        os="arch"; pkg="pacman"; update_cmd="sudo pacman -Sy"; install_cmd="sudo pacman -S --noconfirm" ;;
      * )
        log_error "Unsupported Linux distribution: ${ID}"
        exit 1
        ;;
    esac
  elif [[ "$(uname -s)" == "Darwin" ]]; then
    os="macos"; pkg="brew"; update_cmd="brew update"; install_cmd="brew install"
  else
    log_error "Unsupported operating system: $(uname -s)"
    exit 1
  fi
  OS_FAMILY="$os"
  PKG_MANAGER="$pkg"
  PKG_UPDATE_CMD="$update_cmd"
  PKG_INSTALL_CMD="$install_cmd"
}

check_sudo() {
  if [[ "$EUID" -ne 0 ]]; then
    if ! command -v sudo >/dev/null 2>&1; then
      log_error "This installer requires sudo privileges. Install sudo or run as root."
      exit 1
    fi
  fi
}

prompt_yes_no() {
  local prompt="$1"
  if [[ "$NONINTERACTIVE" == "yes" ]]; then
    return 0
  fi
  read -r -p "$prompt [Y/n] " reply
  reply=${reply,,}
  if [[ -z "$reply" || "$reply" == "y" || "$reply" == "yes" ]]; then
    return 0
  fi
  return 1
}

detect_arch() {
  case "$(uname -m)" in
    x86_64)
      ARCH="x86_64" ;;
    amd64)
      ARCH="x86_64" ;;
    arm64|aarch64)
      ARCH="aarch64" ;;
    *)
      log_error "Unsupported architecture: $(uname -m)"
      exit 1
      ;;
  esac
}

check_existing_install() {
  if command -v volar >/dev/null 2>&1 && [[ "$INSTALL_FORCE" != "yes" ]]; then
    log_info "Volant CLI appears to be installed already (use --force to reinstall)."
    exit 0
  fi
}

ensure_dependencies() {
  local base_packages=(curl tar)
  local missing_packages=()
  local dependencies=(cloud-hypervisor qemu-utils bridge-utils iptables)
  if command -v sha256sum >/dev/null 2>&1; then
    :
  elif command -v shasum >/dev/null 2>&1; then
    :
  else
    base_packages+=(sha256sum)
  fi

  for pkg in "${base_packages[@]}"; do
    if ! command -v "$pkg" >/dev/null 2>&1; then
      missing_packages+=("$pkg")
    fi
  done

  for dep in "${dependencies[@]}"; do
    if ! command -v "$dep" >/dev/null 2>&1; then
      missing_packages+=("$dep")
    fi
  done

  if [[ "${#missing_packages[@]}" -gt 0 ]]; then
    log_warn "The following packages will be installed: ${missing_packages[*]}"
    if prompt_yes_no "Proceed with package installation?"; then
      if [[ -n "$PKG_UPDATE_CMD" ]]; then
        log_info "Updating package index..."
        eval "$PKG_UPDATE_CMD"
      fi
      log_info "Installing dependencies using $PKG_MANAGER..."
      eval "$PKG_INSTALL_CMD ${missing_packages[*]}"
    else
      log_error "Cannot continue without required dependencies."
      exit 1
    fi
  fi
}

create_temp_dir() {
  TMP_DIR=$(mktemp -d -t volant-install-XXXXXX)
}

resolve_version() {
  if [[ "$INSTALL_VERSION" != "latest" ]]; then
    RESOLVED_VERSION="$INSTALL_VERSION"
    return
  fi
  local api="https://api.github.com/repos/ccheshirecat/volant/releases/latest"
  if ! RESOLVED_VERSION=$(curl -sSL "$api" | grep -m1 '"tag_name"' | cut -d '"' -f4); then
    log_error "Unable to determine latest release. Set volant_VERSION manually."
    exit 1
  fi
  if [[ -z "$RESOLVED_VERSION" ]]; then
    log_error "GitHub API did not return a tag."
    exit 1
  fi
}

download_artifact() {
  local name="$1"
  local url="$2"
  local dest="$3"
  log_info "Downloading $name"
  curl -fL "$url" -o "$dest"
}

install_binaries() {
  local archive="${TMP_DIR}/volant-${ARCH}.tar.gz"
  local checksum_file="${TMP_DIR}/checksums.txt"
  local base_url="https://github.com/ccheshirecat/volant/releases/download/${RESOLVED_VERSION}"

  download_artifact "checksums" "${base_url}/checksums.txt" "$checksum_file"
  download_artifact "volant archive" "${base_url}/volant-${ARCH}.tar.gz" "$archive"

  log_info "Verifying checksums..."
  pushd "$TMP_DIR" >/dev/null
  local checksum_line
  checksum_line=$(grep "volant-${ARCH}.tar.gz" checksums.txt || true)
  if [[ -z "$checksum_line" ]]; then
    log_error "Checksum entry for volant-${ARCH}.tar.gz not found."
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    echo "$checksum_line" | sha256sum -c -
  else
    echo "$checksum_line" | shasum -a 256 -c -
  fi
  popd >/dev/null

  log_info "Extracting binaries..."
  tar -xzf "$archive" -C "$TMP_DIR"

  log_info "Installing Volant binaries..."
  if [[ -f "${TMP_DIR}/volar" ]]; then
    sudo install -m 0755 "${TMP_DIR}/volar" /usr/local/bin/volar
  elif [[ -f "${TMP_DIR}/volant" ]]; then
    # Backward-compatibility: older archives used the 'volant' filename for the CLI
    sudo install -m 0755 "${TMP_DIR}/volant" /usr/local/bin/volar
  else
    log_error "CLI binary (volar) not found in archive"
    exit 1
  fi
  if [[ -f "${TMP_DIR}/volantd" ]]; then
    sudo install -m 0755 "${TMP_DIR}/volantd" /usr/local/bin/volantd
  fi
  if [[ -f "${TMP_DIR}/volary" ]]; then
    sudo install -m 0755 "${TMP_DIR}/volary" /usr/local/bin/volary
  fi
}

run_volant_setup() {
  if [[ "$RUN_SETUP" == "no" ]]; then
    log_info "Skipping 'volar setup' as requested."
    return
  fi
  if prompt_yes_no "Run 'sudo volar setup' now?"; then
    log_info "Running 'sudo volar setup'..."
    local kernel_flags=( )
    local env_prefix=( VOLANT_WORK_DIR="$WORK_DIR" )
    if [[ -f "$BZIMAGE_PATH" ]]; then
      kernel_flags+=( --bzimage "$BZIMAGE_PATH" )
      env_prefix+=( VOLANT_KERNEL_BZIMAGE="$BZIMAGE_PATH" )
    else
      log_warn "bzImage not present at $BZIMAGE_PATH; you can provide one later."
    fi
    if [[ -f "$VMLINUX_PATH" ]]; then
      kernel_flags+=( --vmlinux "$VMLINUX_PATH" )
      env_prefix+=( VOLANT_KERNEL_VMLINUX="$VMLINUX_PATH" )
    fi
    if ! sudo "${env_prefix[@]}" volar setup "${kernel_flags[@]}" --work-dir "$WORK_DIR"; then
      log_warn "'volar setup' failed. You can rerun it manually later."
    fi
  else
    log_info "You can run 'sudo volar setup' at any time to initialize the system."
  fi
}

default_kernel_url() {
  # Prefer release-tagged kernel under the repo's kernels directory
  # Expected path: kernels/<arch>/bzImage in the repository at the given tag
  local ref="$RESOLVED_VERSION"
  # Convert 'latest' to 'main' for raw content if resolution failed
  if [[ -z "$ref" || "$ref" == "latest" ]]; then
    ref="main"
  fi
  echo "https://raw.githubusercontent.com/ccheshirecat/volant/${ref}/kernels/${ARCH}/bzImage"
}

provision_kernel() {
  sudo mkdir -p "$KERNEL_DIR"
  # bzImage
  if [[ -f "$BZIMAGE_PATH" ]]; then
    log_info "bzImage already present at $BZIMAGE_PATH"
  else
    local url="$KERNEL_URL"
    if [[ -z "$url" ]]; then
      url=$(default_kernel_url)
    fi
    log_info "Attempting to download bzImage from: $url"
    if curl -fL "$url" -o "$TMP_DIR/bzImage"; then
      sudo install -m 0644 "$TMP_DIR/bzImage" "$BZIMAGE_PATH"
      log_info "bzImage installed to $BZIMAGE_PATH"
    else
      log_warn "Failed to fetch bzImage from $url"
      log_warn "You can place a bzImage at $BZIMAGE_PATH later."
    fi
  fi
  # vmlinux (optional)
  if [[ -n "$VMLINUX_URL" && ! -f "$VMLINUX_PATH" ]]; then
    log_info "Attempting to download vmlinux from: $VMLINUX_URL"
    if curl -fL "$VMLINUX_URL" -o "$TMP_DIR/vmlinux"; then
      sudo install -m 0644 "$TMP_DIR/vmlinux" "$VMLINUX_PATH"
      log_info "vmlinux installed to $VMLINUX_PATH"
    else
      log_warn "Failed to fetch vmlinux from $VMLINUX_URL"
    fi
  fi
}

main() {
  parse_args "$@"
  check_shell
  require_program curl
  render_banner
  check_sudo
  detect_os
  detect_arch
  check_existing_install
  ensure_dependencies
  create_temp_dir
  resolve_version
  install_binaries
  provision_kernel
  run_volant_setup
  log_info "Volant installation complete. Launch with 'volar' or 'volar --help'."
}

main "$@"
