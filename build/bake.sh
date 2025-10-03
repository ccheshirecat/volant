#!/usr/bin/env bash

# Copyright (c) 2025 HYPR. PTE. LTD.
#
# Business Source License 1.1
# See LICENSE file in the project root for details.

set -euo pipefail

# Deterministic initramfs bake script
# - respects SOURCE_DATE_EPOCH for file mtimes
# - uses gzip -n to strip timestamps
# - builds Go and C artifacts without build IDs or paths
# - supports injecting additional files via --copy host_path:guest_path

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)

# Inputs (can be overridden via environment)
BUSYBOX_URL=${BUSYBOX_URL:-"https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"}
BUSYBOX_SHA256=${BUSYBOX_SHA256:-"6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"}
SDE=${SOURCE_DATE_EPOCH:-"0"}

COPIES=()

usage() {
  cat <<EOF
Usage: $0 [--copy host_path:guest_path]...

Options:
  --copy host_path:guest_path   Copy a file or directory from host into the initramfs at guest_path.
                                May be provided multiple times. Directories are copied recursively.

Environment:
  BUSYBOX_URL        URL to busybox static binary (musl). Default: $BUSYBOX_URL
  BUSYBOX_SHA256     Expected SHA-256 of busybox (empty to skip verification)
  SOURCE_DATE_EPOCH  Unix timestamp used for normalized mtimes (default: 0)
EOF
}

# Parse args (supports repeated --copy and --help)
while [ $# -gt 0 ]; do
  case "$1" in
    --copy)
      shift || { echo "--copy requires an argument" >&2; exit 2; }
      COPIES+=("$1")
      ;;
    --copy=*)
      COPIES+=("${1#*=}")
      ;;
    -h|--help)
      usage; exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage; exit 2
      ;;
  esac
  shift || true
done

echo "Using BUSYBOX_URL=$BUSYBOX_URL"
if [[ -n "$BUSYBOX_SHA256" ]]; then
  echo "Expecting BUSYBOX_SHA256=$BUSYBOX_SHA256"
fi
echo "SOURCE_DATE_EPOCH=$SDE"

# Build kestrel deterministically (static CGO off, trim paths, no buildid)
export CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOWORK=off GOFLAGS="-trimpath"
go build -buildvcs=false -ldflags "-s -w -buildid=" -o "$ROOT_DIR/kestrel" ../cmd/kestrel

# Prepare staging directory
WORKDIR=/tmp/initramfs
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"/{bin,sbin,etc,proc,sys,dev,usr/bin,usr/sbin}

# 1) Install deterministic init (static, no build-id)
gcc -static -s -Wl,--build-id=none "$ROOT_DIR/init.c" -o "$WORKDIR/init"
chmod 0755 "$WORKDIR/init"

# 2) Place kestrel
cp "$ROOT_DIR/kestrel" "$WORKDIR/bin/kestrel"
chmod 0755 "$WORKDIR/bin/kestrel"

# 3) BusyBox (pinned download + optional sha256 verification)
curl -fsSL "$BUSYBOX_URL" -o "$WORKDIR/bin/busybox"
if [[ -n "$BUSYBOX_SHA256" ]]; then
  echo "$BUSYBOX_SHA256  $WORKDIR/bin/busybox" | sha256sum -c -
fi
chmod +x "$WORKDIR/bin/busybox"
"$WORKDIR/bin/busybox" --install -s "$WORKDIR/bin"

# 3b) Inject copies requested by user
copy_into_workdir() {
  local host_path="$1" guest_path="$2"
  if [[ -z "$host_path" || -z "$guest_path" ]]; then
    echo "invalid --copy entry: '$host_path:$guest_path'" >&2
    exit 2
  fi
  if [[ ! -e "$host_path" ]]; then
    echo "host path not found: $host_path" >&2
    exit 2
  fi
  # Ensure destination directory exists; strip leading slashes
  local dest="$WORKDIR${guest_path}"
  # Normalize double slashes
  dest="${dest//\/+//}"
  if [[ -d "$host_path" ]]; then
    mkdir -p "$dest"
    # Copy contents of directory into target if guest_path ends with '/'
    if [[ "${guest_path}" == */ ]]; then
      cp -a "$host_path"/. "$dest"/
    else
      # Create parent and copy directory under the specified name
      mkdir -p "$(dirname "$dest")"
      cp -a "$host_path" "$dest"
    fi
  else
    mkdir -p "$(dirname "$dest")"
    cp -a "$host_path" "$dest"
  fi
}

if [[ ${#COPIES[@]} -gt 0 ]]; then
  for item in "${COPIES[@]}"; do
    # Split on first ':'
    src="${item%%:*}"
    dst="${item#*:}"
    if [[ "$src" == "$dst" ]]; then
      echo "--copy expects host_path:guest_path, got '$item'" >&2
      exit 2
    fi
    # Ensure guest path starts with '/'
    if [[ "${dst}" != /* ]]; then
      dst="/${dst}"
    fi
    copy_into_workdir "$src" "$dst"
  done
fi

# 4) Normalize mtimes for reproducibility
find "$WORKDIR" -exec touch -h -d @"$SDE" {} +

# 5) Create the final initramfs archive (gzip -n to drop timestamps)
pushd "$WORKDIR" >/dev/null
find . -print0 | cpio --null -ov --format=newc | gzip -n -9 > "$ROOT_DIR/volant-initramfs.cpio.gz"
popd >/dev/null

echo "Initramfs written to $ROOT_DIR/volant-initramfs.cpio.gz"
echo "Next step: rebuild the Cloud Hypervisor Linux kernel with CONFIG_INITRAMFS_SOURCE pointing to this archive."
echo "Kernel repo: https://github.com/cloud-hypervisor/linux"
echo "Place the resulting bzImage under kernels/x86_64/bzImage in this repo or install to /var/lib/volant/kernel/bzImage on target hosts."
