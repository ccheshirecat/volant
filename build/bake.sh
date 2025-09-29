#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)

gcc -static "$ROOT_DIR/init.c" -o "$ROOT_DIR/init"
pushd "$ROOT_DIR/.." >/dev/null
make build
popd >/dev/null

pushd "$ROOT_DIR" >/dev/null
cp ../bin/volary .
chmod +x fetch-rootfs.sh stage-volary.sh

WORKDIR=/tmp/initramfs
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"/{bin,dev,etc,home,mnt,proc,sys,usr/bin,sbin,usr/sbin,scripts,run,tmp,usr/local/bin,usr/local/sbin,var}
cp init "$WORKDIR"
cp volary "$WORKDIR"/bin
cp volary "$WORKDIR"/usr/local/bin
cp fetch-rootfs.sh stage-volary.sh "$WORKDIR"/scripts

pushd "$WORKDIR" >/dev/null
wget -q -P ./bin https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
chmod +x ./bin/busybox
chroot . /bin/busybox --install -s
ln -sf busybox ./bin/sha256sum >/dev/null 2>&1 || true
find . -print0 | cpio --null -ov --format=newc > initramfs.cpio
gzip -f ./initramfs.cpio
popd >/dev/null

mv "$WORKDIR"/initramfs.cpio.gz "$ROOT_DIR"/volant-initramfs.cpio.gz
echo "Initramfs written to $ROOT_DIR/volant-initramfs.cpio.gz"
