#!/usr/bin/env bash
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CONTEXT="$REPO_ROOT/build/images"
OUTPUT_DIR=$REPO_ROOT/build/artifacts
AGENT_BIN=${1:-$REPO_ROOT/bin/volary}
IMAGE_TAG=${IMAGE_TAG:-volant-initramfs:latest}
INITRAMFS_NAME=${INITRAMFS_NAME:-volant-initramfs.cpio.gz}
KERNEL_URL=${KERNEL_URL:-https://github.com/cloud-hypervisor/linux/releases/download/ch-release-v6.12.8-20250613/vmlinux-x86_64}

mkdir -p "$OUTPUT_DIR"

if [ ! -f "$AGENT_BIN" ]; then
  echo "agent binary not found at $AGENT_BIN" >&2
  exit 1
fi

STAGED_AGENT="$CONTEXT/volary.bin"
TMPDIR=""
CID=""

cleanup_all() {
  rm -f "$STAGED_AGENT" 2>/dev/null || true
  if [ -n "$CID" ]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
  if [ -n "$TMPDIR" ] && [ -d "$TMPDIR" ]; then
    rm -rf "$TMPDIR"
  fi
}
trap cleanup_all EXIT

cp "$AGENT_BIN" "$STAGED_AGENT"

printf 'Building image... ' >&2
if ! docker build --build-arg VOLANT_AGENT_BINARY="$(basename "$STAGED_AGENT")" -t "$IMAGE_TAG" "$CONTEXT" >/dev/null; then
  echo 'failed' >&2
  exit 1
fi
rm -f "$STAGED_AGENT"
echo 'done' >&2

TMPDIR=$(mktemp -d)

CID=$(docker create "$IMAGE_TAG")

docker export "$CID" | tar -C "$TMPDIR" -xf -

pushd "$TMPDIR" >/dev/null
find . | cpio -o -H newc | gzip -9 > "$OUTPUT_DIR/$INITRAMFS_NAME"
popd >/dev/null

docker rm -f "$CID" >/dev/null
CID=""

KERNEL_DEST="$OUTPUT_DIR/vmlinux-x86_64"
if [ ! -f "$KERNEL_DEST" ]; then
  echo "Downloading kernel to $KERNEL_DEST" >&2
  curl -L "$KERNEL_URL" -o "$KERNEL_DEST"
fi

echo "Artifacts written to $OUTPUT_DIR"
