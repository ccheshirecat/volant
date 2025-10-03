#!/usr/bin/env bash

set -euo pipefail

# Default values
INSTALL_VERSION="${VOLANT_VERSION:-latest}"
INSTALL_FORCE=no
RUN_SETUP=yes
NONINTERACTIVE=no
REPO="volantvm/volant"
VMLINUX_URL="https://github.com/cloud-hypervisor/linux/releases/download/ch-release-v6.12.8-20250613/vmlinux-x86_64"

# Install paths
WORK_DIR="/var/lib/volant"
KERNEL_DIR="${WORK_DIR}/kernel"
BZIMAGE_PATH="${KERNEL_DIR}/bzImage"
VMLINUX_PATH="${KERNEL_DIR}/vmlinux"

# Globals
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TMP_DIR=""
OS_FAMILY=""
PKG_MANAGER=""
PKG_UPDATE_CMD=""
PKG_INSTALL_CMD=""
ARCH=""
RESOLVED_VERSION=""

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

Downloads and installs volant binaries from its GitHub release and the vmlinux
kernel directly from the official Cloud Hypervisor release page.

Usage: install.sh [options]

Options:
  --version <ver>     Install a specific volant version (e.g., v0.1.0, default: latest)
  --force             Reinstall even if volant is already present
  --skip-setup        Skip running 'volar setup' after installation
  --yes, -y           Non-interactive mode (assume yes to prompts)
  --help, -h          Show this message
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
    exit 1
  fi
}

check_shell() {
  if [[ -z "${BASH_VERSION:-}" ]]; then
    log_error "This installer must be run with bash. Use 'bash install.sh'."
    exit 1
  fi
}

detect_os() {
  if [[ -f /etc/os-release ]]; then
    # shellcheck source=/dev/null
    . /etc/os-release
    case "$ID" in
      ubuntu|debian)
        OS_FAMILY="debian"; PKG_MANAGER="apt"; PKG_UPDATE_CMD="sudo apt-get update"; PKG_INSTALL_CMD="sudo apt-get install -y" ;;
      fedora)
        OS_FAMILY="fedora"; PKG_MANAGER="dnf"; PKG_UPDATE_CMD="sudo dnf makecache"; PKG_INSTALL_CMD="sudo dnf install -y" ;;
      centos|rhel)
        OS_FAMILY="rhel"; PKG_MANAGER="yum"; PKG_UPDATE_CMD="sudo yum makecache"; PKG_INSTALL_CMD="sudo yum install -y" ;;
      arch)
        OS_FAMILY="arch"; PKG_MANAGER="pacman"; PKG_UPDATE_CMD="sudo pacman -Sy"; PKG_INSTALL_CMD="sudo pacman -S --noconfirm" ;;
      * )
        log_error "Unsupported Linux distribution: ${ID}"
        exit 1
        ;;
    esac
  else
    log_error "Unsupported operating system. This script is for Linux."
    exit 1
  fi
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
    x86_64|amd64)
      ARCH="x86_64" ;;
    *)
      log_error "Unsupported architecture: $(uname -m). Only x86_64 is supported."
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
  local missing_packages=()
  local dependencies=(cloud-hypervisor qemu-utils bridge-utils iptables coreutils)

  for dep in "${dependencies[@]}"; do
    local bin_name="$dep"
    if [[ "$dep" == "qemu-utils" ]]; then bin_name="qemu-img"; fi
    if [[ "$dep" == "bridge-utils" ]]; then bin_name="brctl"; fi
    if [[ "$dep" == "coreutils" ]]; then bin_name="sha256sum"; fi

    if ! command -v "$bin_name" >/dev/null 2>&1; then
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
    log_info "Installing specified volant version: ${RESOLVED_VERSION}"
    return
  fi

  log_info "Finding latest volant release version..."
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"

  local api_response
  if ! api_response=$(curl --fail -sSL "$api_url"); then
      log_error "Failed to contact GitHub API. Please check network or specify a version with --version."
      exit 1
  fi

  local latest_tag
  latest_tag=$(echo "$api_response" | grep -m1 '"tag_name"' | cut -d '"' -f4)

  if [[ -z "$latest_tag" ]]; then
    log_error "GitHub API did not return a valid tag. Please specify a version with --version."
    exit 1
  fi

  RESOLVED_VERSION="$latest_tag"
  log_info "Latest volant version is: ${RESOLVED_VERSION}"
}

download_and_install_artifacts() {
  local base_url="https://github.com/${REPO}/releases/download/${RESOLVED_VERSION}"
  local artifacts=("volar" "kestrel" "volantd" "bzImage" "checksums.txt")

  log_info "Downloading volant artifacts from release ${RESOLVED_VERSION}..."
  for artifact in "${artifacts[@]}"; do
    log_info "Downloading ${artifact}..."
    if ! curl -fL "${base_url}/${artifact}" -o "${TMP_DIR}/${artifact}"; then
      log_error "Failed to download ${artifact}. Please check release assets for version ${RESOLVED_VERSION}."
      exit 1
    fi
  done

  log_info "Verifying volant checksums..."
  pushd "$TMP_DIR" >/dev/null
  if ! sha256sum -c --strict checksums.txt; then
    log_error "Checksum verification failed! Files are corrupted or have been tampered with."
    popd >/dev/null
    exit 1
  fi
  popd >/dev/null
  log_info "Volant checksums verified successfully."

  log_info "Downloading vmlinux kernel from Cloud Hypervisor..."
  if ! curl -fL "${VMLINUX_URL}" -o "${TMP_DIR}/vmlinux"; then
      log_error "Failed to download vmlinux from ${VMLINUX_URL}."
      exit 1
  fi

  log_info "Installing binaries to /usr/local/bin..."
  sudo install -m 0755 "${TMP_DIR}/volar" /usr/local/bin/volar
  sudo install -m 0755 "${TMP_DIR}/kestrel" /usr/local/bin/kestrel
  sudo install -m 0755 "${TMP_DIR}/volantd" /usr/local/bin/volantd

  log_info "Installing kernel files to ${KERNEL_DIR}..."
  sudo mkdir -p "$KERNEL_DIR"
  sudo install -m 0644 "${TMP_DIR}/bzImage" "$BZIMAGE_PATH"
  sudo install -m 0644 "${TMP_DIR}/vmlinux" "$VMLINUX_PATH"
}

run_volant_setup() {
  if [[ "$RUN_SETUP" == "no" ]]; then
    log_info "Skipping 'volar setup' as requested."
    return
  fi

  log_info "The installer needs to run 'sudo volar setup' to configure the system."
  if prompt_yes_no "Run setup now?"; then
    log_info "Running 'sudo volar setup'..."
    local setup_cmd=(sudo volar setup --work-dir "$WORK_DIR")
    if [[ -f "$VMLINUX_PATH" ]]; then
      setup_cmd+=(--vmlinux "$VMLINUX_PATH")
    fi

    if ! "${setup_cmd[@]}"; then
      log_warn "'volar setup' failed. You can rerun it manually later."
    fi
  else
    log_info "Skipping setup. You can run 'sudo volar setup' at any time to initialize the system."
  fi
}

main() {
  parse_args "$@"
  check_shell
  require_program curl
  check_sudo
  detect_os
  detect_arch
  check_existing_install
  ensure_dependencies
  create_temp_dir
  resolve_version
  download_and_install_artifacts
  run_volant_setup
  log_info "✅ Volant installation complete!"
  log_info "   Run 'volar --help' to get started."
}

main "$@"
