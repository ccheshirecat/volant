#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)

# Build volary with CGO disabled
CGO_ENABLED=0 go build -ldflags "-s -w" -o "$ROOT_DIR/volary" ../cmd/volary

# Prepare staging directory
WORKDIR=/tmp/initramfs
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"/{bin,sbin,etc,proc,sys,dev,usr/bin,usr/sbin}

# 1. Install our Go binary as the init process
gcc -static -s "$ROOT_DIR/init.c" -o "$WORKDIR/init"

cp "$ROOT_DIR/volary" "$WORKDIR/bin/volary"
chmod 0755 "$WORKDIR/init"

cd "$WORKDIR"
# 2. Install busybox to provide switch_root and a rescue shell
pushd "$WORKDIR" >/dev/null
wget -q -O ./bin/busybox https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
chmod +x ./bin/busybox

# 3. Create the necessary symlinks for busybox commands (like switch_root)
./bin/busybox --install -s ./bin
popd >/dev/null

# 4. Create the final initramfs archive
pushd "$WORKDIR" >/dev/null
find . -print0 | cpio --null -ov --format=newc | gzip -9 > "$ROOT_DIR/volant-initramfs.cpio.gz"
popd >/dev/null

echo "Initramfs written to $ROOT_DIR/volant-initramfs.cpio.gz"
echo "Next step: rebuild the Cloud Hypervisor Linux kernel with CONFIG_INITRAMFS_SOURCE pointing to this archive."
echo "Kernel repo: https://github.com/cloud-hypervisor/linux"
echo "Place the resulting bzImage under kernels/<arch>/bzImage in this repo or install to /var/lib/volant/kernel/bzImage on target hosts."
