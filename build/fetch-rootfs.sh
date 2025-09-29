#!/bin/sh
set -eu

usage() {
    echo "usage: fetch-rootfs.sh <source> <destination> [checksum]" >&2
    exit 1
}

[ $# -ge 2 ] || usage

SRC=$1
DEST=$2
CHECKSUM=""
if [ $# -ge 3 ]; then
    CHECKSUM=$3
fi

mkdir -p "$(dirname "$DEST")"

case "$SRC" in
    http://*|https://*)
        if command -v curl >/dev/null 2>&1; then
            curl -fsSL "$SRC" -o "$DEST"
        else
            wget -O "$DEST" "$SRC"
        fi
        ;;
    file://*)
        cp "${SRC#file://}" "$DEST"
        ;;
    /*)
        cp "$SRC" "$DEST"
        ;;
    *)
        if [ -f "$SRC" ]; then
            cp "$SRC" "$DEST"
        else
            echo "fetch-rootfs: unsupported source $SRC" >&2
            exit 1
        fi
        ;;
esac

sync "$DEST"

if [ -n "$CHECKSUM" ]; then
    echo "$CHECKSUM  $DEST" | sha256sum -c - >/dev/null 2>&1 || {
        echo "fetch-rootfs: checksum verification failed" >&2
        exit 1
    }
fi

