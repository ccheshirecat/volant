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

💡 Need help deploying or extending this? → hello@volantvm.com

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
- **`kestrel`** — In-guest agent & init (PID 1)

- **[`fledge`](https://github.com/volantvm/fledge)** — Plugin builder (OCI images → bootable artifacts)

**Two paths, same workflow**:

1. **[`Rootfs strategy`](https://github.com/volantvm/oci-plugin-example)** — Convert OCI images to bootable disk images (Docker compatibility)
2. **[`Initramfs strategy`](https://github.com/volantvm/initramfs-plugin-example)** — Build custom appliances from scratch (maximum performance)

---

## Quick Start

```bash
# Install Volant toolchain
curl -fsSL https://get.volantvm.com | bash

# Configure host (bridge network, NAT, systemd)
sudo volar setup

# Install a pre-built plugin from a manifest URL, see the example plugin repositories for more details
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

Use **[fledge](https://github.com/volantvm/fledge)** to build custom plugins from OCI images or static binaries.

**Examples**:
- [initramfs-plugin-example](https://github.com/volantvm/initramfs-plugin-example) — Caddy web server (fast boot, minimal size)
- [oci-plugin-example](https://github.com/volantvm/oci-plugin-example) — NGINX from Docker image (Docker compatibility)

### Install & Run Pre-Built Plugins

Both examples include GitHub Actions workflows that build artifacts automatically. Install directly from their manifests:

```bash
# Initramfs example (Caddy)
volar plugins install --manifest https://raw.githubusercontent.com/volantvm/initramfs-plugin-example/main/manifest/caddy.json
volar vms create web --plugin caddy --cpu 1 --memory 512

# OCI example (NGINX)
volar plugins install --manifest https://raw.githubusercontent.com/volantvm/oci-plugin-example/main/manifest/nginx.json
volar vms create web --plugin nginx --cpu 2 --memory 1024
```

Volant downloads the pre-built artifacts (from GitHub releases) and boots your VM immediately.

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

**Full documentation**: [docs.volantvm.com](https://docs.volantvm.com)

Quick links:
- [Installation Guide](docs/2_getting-started/1_installation.md)
- [Plugin Development](docs/4_plugin-development/1_overview.md)
- [Architecture Overview](docs/5_architecture/1_overview.md)
- [Contributing](docs/7_development/1_contributing.md)

---

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full vision.

- **Immediate future** (2025 Q3-Q4): **VFIO GPU passthrough**
- **Mid-term+** (Early 2026): PaaS mode, Multi-node clustering

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


---

**© 2025 HYPR PTE. LTD.**
