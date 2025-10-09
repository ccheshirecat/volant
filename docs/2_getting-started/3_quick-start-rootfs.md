# Quick Start: OCI Rootfs Strategy

Run unmodified Docker/OCI images as microVMs with hardware isolation.

Verified against code paths that support rootfs boot:
- internal/pluginspec/spec.go (RootFS fields)
- Orchestrator boot flow

## Install a prebuilt OCI plugin

```bash
volar plugins install --manifest \
  https://raw.githubusercontent.com/volantvm/oci-plugin-example/main/manifest/nginx.json

volar vms create web --plugin nginx --cpu 2 --memory 1024
```

Result: a microVM that boots in seconds with an OCI-based root filesystem.

## Build your own (with Fledge)

Install Fledge if you haven't already:

```bash
curl -LO https://github.com/volantvm/fledge/releases/latest/download/fledge-linux-amd64
chmod +x fledge-linux-amd64 && sudo mv fledge-linux-amd64 /usr/local/bin/fledge
```

### Option A: fledge.toml workflow

```toml
# fledge.toml (OCI)
version = "1"
strategy = "oci_rootfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
image = "docker://nginx:alpine"

[filesystem]
type = "ext4"
size_buffer_mb = 100
preallocate = false
```

```bash
sudo fledge build
# → outputs nginx-rootfs.img and nginx.manifest.json

volar plugins install --manifest nginx.manifest.json
volar vms create demo --plugin nginx
```

You can swap `source.image` for a Dockerfile build by providing `source.dockerfile` (plus optional `context`, `target`, and `build_args`).

### Option B: build directly from a Dockerfile

Skip the config file and point Fledge at a Dockerfile:

```bash
sudo fledge build ./Dockerfile \
  --context . \
  --target runtime-stage \
  --build-arg FOO=bar
# → outputs <directory>.img + <directory>.manifest.json

volar plugins install --manifest <directory>.manifest.json
```

Use `--output <name>` to override the artifact prefix. All direct-build flags (`--context`, `--target`, `--build-arg`, `--output`, `--output-initramfs`) map to the embedded BuildKit pipeline inside Fledge.

See also: docs/4_plugin-development/3_oci-rootfs.md for deeper coverage.
