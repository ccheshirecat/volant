#!/usr/bin/env bash

set -euo pipefail

INSTALL_VERSION="${overhyped_VERSION:-latest}"
INSTALL_FORCE=no
RUN_SETUP=yes
NONINTERACTIVE=no

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
overhyped Installer

Usage: install.sh [options]

Options:
  --version <ver>     Install a specific version (default: latest)
  --force             Reinstall even if overhyped is already present
  --skip-setup        Skip running 'hype setup' after installation
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
    log_info "overhyped Installer"
    return
  fi
  local font="terrace"
  if ! require_figlet_font "$font"; then
    font="pagga"
  fi
  if ! require_figlet_font "$font"; then
    log_info "overhyped Installer"
    return
  fi
  figlet -f "$font" "overhyped"
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
  if command -v overhyped >/dev/null 2>&1 && [[ "$INSTALL_FORCE" != "yes" ]]; then
    log_info "overhyped appears to be installed already (use --force to reinstall)."
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
  TMP_DIR=$(mktemp -d -t overhyped-install-XXXXXX)
}

resolve_version() {
  if [[ "$INSTALL_VERSION" != "latest" ]]; then
    RESOLVED_VERSION="$INSTALL_VERSION"
    return
  fi
  local api="https://api.github.com/repos/ccheshirecat/overhyped/releases/latest"
  if ! RESOLVED_VERSION=$(curl -sSL "$api" | grep -m1 '"tag_name"' | cut -d '"' -f4); then
    log_error "Unable to determine latest release. Set overhyped_VERSION manually."
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
  local archive="${TMP_DIR}/overhyped-${ARCH}.tar.gz"
  local checksum_file="${TMP_DIR}/checksums.txt"
  local base_url="https://github.com/ccheshirecat/overhyped/releases/download/${RESOLVED_VERSION}"

  download_artifact "checksums" "${base_url}/checksums.txt" "$checksum_file"
  download_artifact "overhyped archive" "${base_url}/overhyped-${ARCH}.tar.gz" "$archive"

  log_info "Verifying checksums..."
  pushd "$TMP_DIR" >/dev/null
  local checksum_line
  checksum_line=$(grep "overhyped-${ARCH}.tar.gz" checksums.txt || true)
  if [[ -z "$checksum_line" ]]; then
    log_error "Checksum entry for overhyped-${ARCH}.tar.gz not found."
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

  log_info "Installing overhyped binaries..."
  sudo install -m 0755 "${TMP_DIR}/overhyped" /usr/local/bin/hype
  if [[ -f "${TMP_DIR}/hype-agent" ]]; then
    sudo install -m 0755 "${TMP_DIR}/hype-agent" /usr/local/bin/hype-agent
  fi
}

run_overhyped_setup() {
  if [[ "$RUN_SETUP" == "no" ]]; then
    log_info "Skipping 'hype setup' as requested."
    return
  fi
  if prompt_yes_no "Run 'sudo hype setup' now?"; then
    log_info "Running 'sudo hype setup'..."
    if ! sudo hype setup; then
      log_warn "'hype setup' failed. You can rerun it manually later."
    fi
  else
    log_info "You can run 'sudo hype setup' at any time to initialize the system."
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
  run_overhyped_setup
  log_info "Overhyped installation complete. Launch with 'hype' or 'hype --help'."
}

main "$@"
