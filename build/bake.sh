#!/usr/bin/env bash
set -euo pipefail

# Deterministic initramfs bake script
# - respects SOURCE_DATE_EPOCH for file mtimes
# - uses gzip -n to strip timestamps
# - builds Go and C artifacts without build IDs or paths

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)

# Inputs (can be overridden via environment)
BUSYBOX_URL=${BUSYBOX_URL:-"https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"}
BUSYBOX_SHA256=${BUSYBOX_SHA256:-""}
SDE=${SOURCE_DATE_EPOCH:-"0"}

echo "Using BUSYBOX_URL=$BUSYBOX_URL"
if [[ -n "$BUSYBOX_SHA256" ]]; then
  echo "Expecting BUSYBOX_SHA256=$BUSYBOX_SHA256"
fi
echo "SOURCE_DATE_EPOCH=$SDE"

# Build volary deterministically (static CGO off, trim paths, no buildid)
export CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOWORK=off GOFLAGS="-trimpath"
go build -buildvcs=false -ldflags "-s -w -buildid=" -o "$ROOT_DIR/volary" ../cmd/volary

# Prepare staging directory
WORKDIR=/tmp/initramfs
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"/{bin,sbin,etc,proc,sys,dev,usr/bin,usr/sbin}

# 1) Install deterministic init (static, no build-id)
gcc -static -s -Wl,--build-id=none "$ROOT_DIR/init.c" -o "$WORKDIR/init"
chmod 0755 "$WORKDIR/init"

# 2) Place volary
cp "$ROOT_DIR/volary" "$WORKDIR/bin/volary"
chmod 0755 "$WORKDIR/bin/volary"

# 3) BusyBox (pinned download + optional sha256 verification)
curl -fsSL "$BUSYBOX_URL" -o "$WORKDIR/bin/busybox"
if [[ -n "$BUSYBOX_SHA256" ]]; then
  echo "$BUSYBOX_SHA256  $WORKDIR/bin/busybox" | sha256sum -c -
fi
chmod +x "$WORKDIR/bin/busybox"
"$WORKDIR/bin/busybox" --install -s "$WORKDIR/bin"

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
