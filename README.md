<p align="center">
  <img src="banner.png" alt="VOLANT — The Intelligent Execution Cloud"/>
</p>

<p align="center">
  <a href="https://github.com/volantvm/volant/actions">
    <img src="https://img.shields.io/github/actions/workflow/status/volantvm/volant/ci.yml?branch=main&style=flat-square&label=tests" alt="Build Status">
  </a>
  <a href="https://github.com/volantvm/volant/releases">
    <img src="https://img.shields.io/github/v/release/volantvm/volant.svg?style=flat-square" alt="Latest Release">
  </a>
  <a href="https://golang.org/">
    <img src="https://img.shields.io/badge/Go-1.22+-black.svg?style=flat-square" alt="Go Version">
  </a>
  <a href="https://github.com/volantvm/volant/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/License-BSL_1.1-black.svg?style=flat-square" alt="License">
  </a>
</p>

---

# Volant

> **The modular microVM orchestration engine.**

Volant turns microVMs into a first-class runtime surface. The project ships a control plane, CLI, and agent that speak a common plugin manifest so teams can run secure, stateful workloads without stitching together networking, scheduling, and lifecycle plumbing themselves.

Runtime-specific behavior lives in signed manifests and their associated artifacts. The core engine stays lean while plugin authors ship the kernels/initramfs overlays and workload processes their runtime requires. Operators decide which manifests to install and must reference one whenever a VM is created.

---

## Overview

Volant provides:

- **`volantd`** — Control plane (SQLite registry + VM orchestration)
- **`volar`** — CLI for managing VMs and plugins
- **`kestrel`** — In-guest agent (PID 1)
- **[`fledge`](https://github.com/volantvm/fledge)** — Plugin builder (OCI images → bootable artifacts)

**Two paths, same workflow**:

1. **Rootfs strategy** — Convert OCI images to bootable disk images (Docker compatibility)
2. **Initramfs strategy** — Build custom appliances from scratch (maximum performance)

---

## Quick Start

```bash
# Install Volant toolchain
curl -fsSL https://get.volantvm.com | bash

# Configure host (bridge network, NAT, systemd)
sudo volar setup

# Install a pre-built plugin from a manifest URL
volar plugins install --manifest https://raw.githubusercontent.com/volantvm/initramfs-plugin-example/main/manifest/caddy.json

# Create and run a VM
volar vms create web --plugin caddy --cpu 2 --memory 512
volar vms list

# Scale declaratively with deployments
cat > web-config.json <<EOF
{
  "plugin": "caddy",
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 512
  }
}
EOF

volar deployments create web-cluster --config web-config.json --replicas 5
```

**Result**: 5 isolated VMs, each with its own kernel and dedicated IP address.

### Build Your Own Plugin

For custom workloads, use `fledge` to build plugins:

```bash
# Option 1: From an OCI image
cat > fledge.toml <<EOF
[plugin]
name = "myapp"
strategy = "oci_rootfs"

[oci_source]
image = "docker.io/library/nginx:alpine"
EOF

fledge build
volar plugins install --manifest myapp.manifest.json

# Option 2: Custom initramfs appliance
cat > fledge.toml <<EOF
[plugin]
name = "myapp"
strategy = "initramfs"

[[file_mappings]]
source = "./mybinary"
dest = "/usr/local/bin/mybinary"
mode = 0o755

[workload]
entrypoint = ["/usr/local/bin/mybinary"]
EOF

fledge build
volar plugins install --manifest myapp.manifest.json
```

---

## Why Volant?

| Feature | Containers | Volant microVMs |
|---------|-----------|----------------|
| **Isolation** | Kernel shared | Hardware-level (dedicated kernel) |
| **Boot time** | ~1s | 50-150ms (initramfs) / 2-5s (rootfs) |
| **Image size** | 197 MB (NGINX) | 12 MB (full appliance) |
| **Security** | Namespaces | Full VM isolation |
| **Overhead** | Shared kernel | ~25 MB per VM |
| **Networking** | NAT/bridge/overlay | Simple Linux bridge |

---

## Architecture

```
┌─────────────────────────────────────────┐
│              Host Machine               │
│  ┌───────────────────────────────────┐  │
│  │    volantd (Control Plane)        │  │
│  │  • SQLite registry                │  │
│  │  • IPAM (192.168.127.0/24)        │  │
│  │  • Cloud Hypervisor orchestration │  │
│  │  • REST + MCP APIs                │  │
│  └───────────────┬───────────────────┘  │
│                  │                       │
│  ┌───────────────▼───────────────────┐  │
│  │     Bridge Network (vbr0)         │  │
│  └┬────────┬────────┬────────┬───────┘  │
│   │        │        │        │           │
│  ┌▼──┐   ┌▼──┐   ┌▼──┐   ┌▼──┐         │
│  │VM1│   │VM2│   │VM3│   │VMN│         │
│  │┌──┐   │┌──┐   │┌──┐   │┌──┐         │
│  │││   │││   │││   │││         │
│  │└──┘   │└──┘   │└──┘   │└──┘         │
│  └───┘   └───┘   └───┘   └───┘         │
│   kestrel agents (PID 1)                │
└─────────────────────────────────────────┘
```

**Dual-kernel design**:
- `bzImage` — For rootfs (baked-in initramfs bootloader)
- `vmlinux` — For custom initramfs (pristine kernel)

---

## Use Cases

-  **Secure multi-tenancy** — True hardware isolation
-  **Edge computing** — Minimal footprint, fast boot
-  **CI/CD** — Ephemeral test environments
-  **Development** — Local Kubernetes-style orchestration
-  **High-density workloads** — 50-100 VMs per host

---

## Documentation

**Start here**: [docs/1_introduction.md](docs/1_introduction.md)

### Getting Started
- [Installation](docs/2_getting-started/1_installation.md)
- [Quick Start: Rootfs (NGINX)](docs/2_getting-started/2_quick-start-rootfs.md)
- [Quick Start: Initramfs (Caddy)](docs/2_getting-started/3_quick-start-initramfs.md)

### Guides
- CLI Reference
- Plugin Development
- Networking
- Scaling
- Cloud-init
- Interactive Shell

### Architecture
- [Overview](docs/5_architecture/1_overview.md)
- Boot Process
- Control Plane Internals
- Security Model

### Reference
- Plugin Manifest Schema
- REST API
- MCP Protocol
- Glossary

### Development
- Contributing Guide
- Building from Source

---

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full vision.

**Highlights**:
- **v0.2-0.3** (2025 Q1-Q2): Testing, observability, security hardening, web dashboard
- **v0.4-0.6** (2025 Q2-Q4): **VFIO GPU passthrough**, **PaaS mode** (Heroku-style `git push`)
- **v1.0+** (2026+): Multi-node clustering, plugin marketplace, enterprise features

---

## Community

-  **GitHub**: [github.com/volantvm/volant](https://github.com/volantvm/volant)
-  **Discord**: *(coming soon)*
-  **Email**: hello@volantvm.com

**Contributing**: See [docs/7_development/1_contributing.md](docs/7_development/1_contributing.md)

---

## License

**Business Source License 1.1** — Free for non-production use.
Converts to **Apache 2.0** on **October 4, 2029**.

See [LICENSE](LICENSE) for full terms.

---

<p align="center">
  <strong>Volant</strong> — <em>Designed for stealth, speed, and scale.</em>
</p>
