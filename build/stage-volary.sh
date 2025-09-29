#!/bin/sh
set -eu

TARGET=${1:-/sysroot}
SRC=/bin/volary
DEST=$TARGET/usr/local/bin/volary
ALT_DEST=$TARGET/bin/volary

mkdir -p "$(dirname "$DEST")"
cp "$SRC" "$DEST"
chmod 0755 "$DEST"

mkdir -p "$(dirname "$ALT_DEST")"
cp "$SRC" "$ALT_DEST"
chmod 0755 "$ALT_DEST"

# Optional: seed configs, ensure /var/run, etc.
mkdir -p "$TARGET/run" "$TARGET/tmp"
