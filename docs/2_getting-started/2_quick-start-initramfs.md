# Quick Start: Initramfs Strategy

This path builds ultra-fast, minimal appliances. Verified against:
- internal/pluginspec/spec.go (initramfs semantics)
- initramfs-plugin-example repository

## Install a prebuilt initramfs plugin

```bash
volar plugins install --manifest \
  https://raw.githubusercontent.com/volantvm/initramfs-plugin-example/main/manifest/caddy.json

volar vms create web --plugin caddy --cpu 1 --memory 512
```

Result: a microVM that boots in ~100ms and serves HTTP on its assigned IP.

## Build your own (with Fledge)

```bash
# Install fledge
curl -LO https://github.com/volantvm/fledge/releases/latest/download/fledge-linux-amd64
chmod +x fledge-linux-amd64 && sudo mv fledge-linux-amd64 /usr/local/bin/fledge

# Minimal fledge.toml
cat > fledge.toml <<'EOF'
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"

[mappings]
"./myapp" = "/usr/bin/myapp"
EOF

sudo fledge build
# → outputs plugin.cpio.gz and manifest JSON
```

Install and run:
```bash
volar plugins install --manifest myapp.manifest.json
volar vms create demo --plugin myapp
```

See also: docs/4_plugin-development/2_initramfs.md
