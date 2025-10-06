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

Spin up your first microVM in under a minute.

---

### 1. Install the Volant toolchain
```bash
# This installs volar (CLI), volantd (control plane), kestrel (guest agent),
# and default kernels to /var/lib/volant/kernel.
# By default, setup creates a bridge (vbr0) at 192.168.127.1/24.

curl -fsSL https://get.volantvm.com | bash
```

**Tip:** To inspect or customize network setup later:
```bash
sudo volar setup --help
```

If you prefer to **skip automatic setup** and handle networking yourself:
```bash
curl -fsSL https://get.volantvm.com | bash -s -- --skip-setup
```

---

### 2. Install a pre-built plugin

Let’s start with a Caddy initramfs plugin [(initramfs-plugin-example)](https://github.com/volantvm/initramfs-plugin-example)

```bash
volar plugins install --manifest \
  https://github.com/volantvm/initramfs-plugin-example/releases/latest/download/caddy.json
```

---

### 3. Create and run your first VM
```bash
volar vms create web --plugin caddy --cpu 2 --memory 512
```

Check it’s alive:
```bash
curl 192.168.127.10
# → Hello from Caddy in a Volant microVM! 🚀
```

---

### 4. Try a Docker-based workload [(oci-plugin-example)](https://github.com/volantvm/oci-plugin-example)

This example runs **NGINX** directly from the official Docker image:
```bash
volar plugins install --manifest \
  https://github.com/volantvm/oci-plugin-example/releases/latest/download/nginx.json

volar vms create my-nginx --plugin nginx --cpu 1 --memory 1024
curl http://192.168.127.11
```

---

### 5. Scale declaratively (Kubernetes-style)

```bash
cat > web-config.json <<'EOF'
{
  "plugin": "caddy",
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 512
  }
}
EOF

volar deployments create web-cluster \
  --config web-config.json \
  --replicas 5
```

**Result:** 5 isolated microVMs, each with its own kernel, IP, and lifecycle management.

---

**Done** — you’ve just deployed a replicated microVM cluster with real kernel isolation, no YAMLs, and zero boilerplate.



### Build Your Own Plugin

Use **[fledge](https://github.com/volantvm/fledge)** to build custom plugins from OCI images or static binaries.

**Examples**:
- [initramfs-plugin-example](https://github.com/volantvm/initramfs-plugin-example) — Caddy web server (fast boot, minimal size)
- [oci-plugin-example](https://github.com/volantvm/oci-plugin-example) — NGINX from Docker image (Docker compatibility)


---

## Why Volant?

| Feature | Containers | Volant microVMs |
|---------|-----------|----------------|
| **Isolation** | Kernel shared | Hardware-level (dedicated kernel) |
| **Boot time** | ~1s | 50-150ms (initramfs) / 2-5s (rootfs) |
| **Image size** | 80 MB (NGINX) | 20 MB (full appliance) |
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

**Unified kernel boot**:
- Defaults to `bzImage` for maximum compatibility
- Optional `vmlinux` override when needed
- Both initramfs and rootfs can be supplied; the agent selects the boot path via `volant.boot` (auto | initramfs | rootfs)

### Web UI and API

- CORS: set `VOLANT_CORS_ORIGINS="http://localhost:3000,https://app.example.com"` to enable browser-based UIs
- IP allowlist: `VOLANT_API_ALLOW_CIDR="127.0.0.1/32,192.168.0.0/16"`
- API key: `VOLANT_API_KEY=...` then send header `X-Volant-API-Key: <key>`
- System summary: `GET /api/v1/system/summary`
- VM list with filters/pagination: `GET /api/v1/vms?status=running&runtime=browser&plugin=caddy&q=web&limit=20&offset=0&sort=created_at&order=desc` (returns `X-Total-Count`)
- Console WebSocket: `GET ws://<host>/ws/v1/vms/:name/console` (raw serial bridge)
- Plugin artifacts API:
  - List: `GET /api/v1/plugins/:plugin/artifacts?version=v1`
  - Upsert: `POST /api/v1/plugins/:plugin/artifacts`
  - Delete: `DELETE /api/v1/plugins/:plugin/artifacts?version=v1`

VM-level device overrides (VFIO):

```json
{
  "devices": {
    "vfio": ["0000:01:00.0", "0000:01:00.1"]
  }
}
```
Apply with `PATCH /api/v1/vms/:name/config` or via `volar` config patching.

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
- [Reference: Manifest](docs/6_reference/1_manifest-schema.md) · [fledge.toml](docs/6_reference/2_fledge-toml.md) · [CLI](docs/6_reference/cli-volar.md) · [OpenAPI](docs/api-reference/openapi.json)
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

**Contributing**: See [contributing]([docs/7_development/1_contributing.md](https://docs.volantvm.com/contributing-1646061m0))

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
