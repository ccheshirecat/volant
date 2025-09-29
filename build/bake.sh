#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$ROOT_DIR/.." && pwd)

# Build the static C init
pushd "$ROOT_DIR" >/dev/null
clang -static init.c -o init
popd >/dev/null

# Build volary with CGO disabled
pushd "$PROJECT_ROOT" >/dev/null
CGO_ENABLED=0 go build -o "$ROOT_DIR/volary" ./cmd/volary
popd >/dev/null

# Prepare staging directory
WORKDIR=/tmp/initramfs
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"/{bin,dev,etc,home,mnt,proc,sys,usr/bin,sbin,usr/sbin,usr/local/bin,var,tmp,run,scripts}

cp "$ROOT_DIR/init" "$WORKDIR"
cp "$ROOT_DIR/volary" "$WORKDIR/bin"
ln -sf ../bin/volary "$WORKDIR/usr/local/bin/volary"
cp "$ROOT_DIR"/fetch-rootfs.sh "$WORKDIR/scripts"
cp "$ROOT_DIR"/stage-volary.sh "$WORKDIR/scripts"
chmod +x "$WORKDIR"/scripts/*

pushd "$WORKDIR" >/dev/null
wget -q -P ./bin https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
chmod +x ./bin/busybox
chroot . /bin/busybox --install -s
ln -sf busybox ./bin/sha256sum >/dev/null 2>&1 || true
find . -print0 | cpio --null -ov --format=newc >../initramfs.cpio
gzip -f ../initramfs.cpio
popd >/dev/null

mv "$WORKDIR/../initramfs.cpio.gz" "$ROOT_DIR/volant-initramfs.cpio.gz"
echo "Initramfs written to $ROOT_DIR/volant-initramfs.cpio.gz"
