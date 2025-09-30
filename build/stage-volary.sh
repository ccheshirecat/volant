#!/bin/sh
set -eu

TARGET=${1:-/sysroot}
SRC=/bin/volary
DEST=$TARGET/usr/local/bin/volary
ALT_DEST=$TARGET/bin/volary
LOG_DIR="$TARGET/var/log"
LOG_FILE="$LOG_DIR/volant-stage.log"

mkdir -p "$LOG_DIR"
{
    echo "[stage-volary] target=$TARGET"
    date -u '+[stage-volary] %Y-%m-%dT%H:%M:%SZ start'
    if [ ! -f "$SRC" ]; then
        echo "[stage-volary] missing source $SRC"
        exit 1
    fi

    mkdir -p "$(dirname "$DEST")"
    cp -f "$SRC" "$DEST"
    chmod 0755 "$DEST"
    echo "[stage-volary] installed volary to $DEST"

    mkdir -p "$(dirname "$ALT_DEST")"
    cp -f "$SRC" "$ALT_DEST"
    chmod 0755 "$ALT_DEST"
    echo "[stage-volary] installed volary to $ALT_DEST"

    mkdir -p "$TARGET/run" "$TARGET/tmp"
    echo "[stage-volary] ensured runtime directories"
    sync "$DEST" "$ALT_DEST" "$TARGET/run" "$TARGET/tmp" 2>/dev/null || true
    date -u '+[stage-volary] %Y-%m-%dT%H:%M:%SZ complete'
} >>"$LOG_FILE" 2>&1
