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

```toml
# fledge.toml (OCI)
version = "1"
strategy = "oci_rootfs"

[oci_source]
image = "nginx:alpine"
```

```bash
sudo fledge build
# → outputs <name>-rootfs.img and manifest JSON

volar plugins install --manifest <name>.manifest.json
volar vms create demo --plugin <name>
```

See also: docs/4_plugin-development/3_oci-rootfs.md
