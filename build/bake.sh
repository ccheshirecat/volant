#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$ROOT_DIR/.." && pwd)

# Build volary with CGO disabled
pushd "$PROJECT_ROOT" >/dev/null
CGO_ENABLED=0 go build -ldflags "-s -w" -o "$ROOT_DIR/volary" ./cmd/volary
popd >/dev/null

# Prepare staging directory
WORKDIR=/tmp/initramfs
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"
cp "$ROOT_DIR/volary" "$WORKDIR/init"
chmod 0755 "$WORKDIR/init"

pushd "$WORKDIR" >/dev/null
find . -print0 | cpio --null -ov --format=newc | gzip -9 > "$ROOT_DIR/volant-initramfs.cpio.gz"
popd >/dev/null

echo "Initramfs written to $ROOT_DIR/volant-initramfs.cpio.gz"
