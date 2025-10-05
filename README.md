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
<p align="center">
  <img src="https://github.com/ccheshirecat/volant-demo/raw/refs/heads/main/demo.gif" width="800" />
</p>


# Volant

> **The modular microVM orchestration engine.**

Volant lets you spin up fully isolated microVMs as easily as running a container — with real kernels, VFIO passthrough, and cloud-init built in.


Volant turns microVMs into a first-class runtime surface. The project ships a control plane, CLI, and agent that speak a common plugin manifest so teams can run secure, stateful workloads without stitching together networking, scheduling, and lifecycle plumbing themselves.

Runtime-specific behavior lives in signed manifests and their associated artifacts. The core engine stays lean while plugin authors ship the kernels/initramfs overlays and workload processes their runtime requires. Operators decide which manifests to install and must reference one whenever a VM is created.

Together with fledge — the artifact builder — Volant provides a complete solution for building and deploying microVM's with custom applications embedded in initramfs, or with the regular OCI images we are familiar with.

Cloud-init support makes Volant ideal for dev sandboxes, while VFIO passthrough allows for isolation of GPU and AI workloads.

## Batteries Included By Default

Volant ships with sensible defaults out of the box, lowering the barrier to entry while keeping full configurability for power users

However, Volant is built to be modular, scriptable and configurable beyond those defaults, and advanced users can customize it to their own needs.

For instance, the Kestrel agent acts as a robust PID1 and is responsible for setting up the guest environment in multiple scenarios, and also acts as a secure proxy to workloads inside network-isolated VM's over vsock, providing a frictionless path to maximum isolation.

If you require more fine-grained control, it is possible to override the kernel paths and the fledge artifact builder has configuration settings for using your own init. Refer to the documentation for more details.

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
# Note: This configures the runtime directory to ~/.volant and sets up a default IPv4 bridge
# use --skip-setup if you want to configure the defaults
# Then run sudo volar setup --help to view the options
curl -fsSL https://get.volantvm.com | bash
```
```bash
# If you want to skip the setup
curl -fsSL https://get.volantvm.com | bash -s -- --skip-setup
```
```bash
# Then run volar setup to view the options for configuration
volar setup
```
      
```bash
# Install a pre-built plugin from a manifest URL, see the example plugin repositories for more details
volar plugins install --manifest https://github.com/volantvm/initramfs-plugin-example/releases/latest/download/caddy.json
```
```bash
# Create and run a VM
volar vms create web --plugin caddy --cpu 2 --memory 512
```
```bash
# Your VM is live
curl 192.168.127.10
```

```bash
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
| **GPU Passthrough** | Limited | Native VFIO for AI/ML |

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
-  **AI/ML** Run machine learning workloads in isolation

---

## Documentation

**Full documentation**: [docs.volantvm.com](https://docs.volantvm.com)

Quick links:
- [Why Volant](docs/1_introduction/0_why-volant.md)
- [Installation Guide](docs/2_getting-started/1_installation.md)
- [Quick Starts](docs/2_getting-started/2_quick-start-initramfs.md) · [Rootfs](docs/2_getting-started/3_quick-start-rootfs.md)
- [Networking](docs/3_guides/1_networking.md) · [Cloud-init](docs/3_guides/2_cloud-init.md) · [Deployments](docs/3_guides/3_deployments.md) · [GPU](docs/3_guides/4_gpu-passthrough.md)
- [Plugin Development](docs/4_plugin-development/1_overview.md) · [Initramfs](docs/4_plugin-development/2_initramfs.md) · [OCI Rootfs](docs/4_plugin-development/3_oci-rootfs.md)
- [Architecture Overview](docs/5_architecture/1_overview.md)
- [Reference: Manifest](docs/6_reference/1_manifest-schema.md) · [fledge.toml](docs/6_reference/2_fledge-toml.md) · [CLI](docs/6_reference/cli-volar.md)
- [Contributing](docs/7_development/1_contributing.md) · [Security](docs/7_development/2_security.md)

---

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full vision.

~~[**2025 Q3-Q4**: **VFIO GPU passthrough**](https://github.com/volantvm/volant/releases/tag/v0.6.0) — Native GPU support for AI/ML workloads~~
- **2025 Q4**: PaaS mode — serverless-like workloads, boot from snapshot
- **2025 Q4**: Multi-node clustering support

---

## Community

-  **GitHub**: [github.com/volantvm/volant](https://github.com/volantvm/volant)
-  **Discord**: *(coming soon)*
-  **Email**: hello@volantvm.com

**Contributing**: See [docs/7_development/1_contributing.md](docs/7_development/1_contributing.md)

---

## License

**Business Source License 1.1** — free for personal, educational, and internal use.
Commercial hosting or resale requires a license from HYPR PTE. LTD.
Converts to **Apache 2.0** on **October 4, 2029**.

See [LICENSE](LICENSE) for full terms.

---

<p align="center">
  <strong>Volant</strong> — <em>Designed for stealth, speed, and scale.</em>
</p>


---

**© 2025 HYPR PTE. LTD.**
