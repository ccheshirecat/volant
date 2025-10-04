# Volant

**Boot microVMs, not containers. Run services, not orchestration.**

Volant is a lightweight microVM orchestrator that solves the "197 MB NGINX" problem. Instead of bloated containers, Volant turns your applications into purpose-built bootable appliances with sub-100ms boot times and minimal memory footprints.

---

## The Problem

```bash
$ docker images nginx:alpine
REPOSITORY    TAG      IMAGE ID       SIZE
nginx         alpine   3b25b682ea82   197MB

# For NGINX. A web server. In 2025.
```

What you *actually* want running:
- A kernel (~10 MB)
- The NGINX binary (~1 MB)
- Your static content
- **Total: ~12 MB, boots in 80 ms**

---

## The Solution

Volant provides:

- **`volantd`** — Control plane (SQLite registry + VM orchestration)
- **`volar`** — CLI for humans
- **`kestrel`** — In-guest agent (PID 1)
- **`fledge`** — Plugin builder (OCI → bootable images)

**Two paths, same workflow**:

1. **Rootfs strategy** — Convert OCI images to bootable disk images (Docker compatibility)
2. **Initramfs strategy** — Build custom appliances from scratch (maximum performance)

---

## Quick Start

```bash
# Install Volant toolchain
curl -fsSL https://volant.sh/install | bash

# Configure host (bridge network, NAT, systemd)
sudo volar setup

# Option 1: Use an OCI image (rootfs)
cat > fledge.toml <<EOF
[plugin]
name = "nginx"
version = "1.0.0"
strategy = "oci_rootfs"

[oci_source]
image = "docker.io/library/nginx:alpine"
EOF

fledge build
volar plugins install --manifest nginx.manifest.json

# Option 2: Build a custom appliance (initramfs)
cat > fledge.toml <<EOF
[plugin]
name = "caddy"
version = "1.0.0"
strategy = "initramfs"

[[file_mappings]]
source = "./caddy_linux_amd64"
dest = "/usr/local/bin/caddy"
mode = 0o755

[[file_mappings]]
source = "./Caddyfile"
dest = "/etc/Caddyfile"
mode = 0o644

[workload]
entrypoint = ["/usr/local/bin/caddy", "run", "--config", "/etc/Caddyfile"]
EOF

fledge build
volar plugins install --manifest caddy.manifest.json

# Create and run a VM
volar vms create web --plugin nginx --cpu 2 --memory 512
volar vms list

# Scale declaratively
volar deployments create web-cluster --plugin nginx --replicas 5
```

**Result**: 5 isolated VMs, each with its own kernel, own IP, <100ms boot time.

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
│  │     Bridge Network (volant0)      │  │
│  └┬────────┬────────┬────────┬───────┘  │
│   │        │        │        │           │
│  ┌▼──┐   ┌▼──┐   ┌▼──┐   ┌▼──┐         │
│  │VM1│   │VM2│   │VM3│   │VMN│         │
│  │┌──┐   │┌──┐   │┌──┐   │┌──┐         │
│  ││🦅│   ││🦅│   ││🦅│   ││🦅│         │
│  │└──┘   │└──┘   │└──┘   │└──┘         │
│  └───┘   └───┘   └───┘   └───┘         │
│   kestrel agents (PID 1)                │
└─────────────────────────────────────────┘
```

**Dual-kernel design**:
- `bzImage-volant` — For rootfs (baked-in initramfs bootloader)
- `vmlinux-generic` — For custom initramfs (pristine kernel)

---

## Use Cases

- ✅ **Secure multi-tenancy** — True hardware isolation
- ✅ **Edge computing** — Minimal footprint, fast boot
- ✅ **CI/CD** — Ephemeral test environments
- ✅ **Development** — Local Kubernetes-style orchestration
- ✅ **High-density workloads** — 50-100 VMs per host

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

- 🐙 **GitHub**: [github.com/ccheshirecat/volant](https://github.com/ccheshirecat/volant)
- 💬 **Discord**: [discord.gg/volant](https://discord.gg/volant) *(coming soon)*
- 📧 **Email**: [email protected]

**Contributing**: See [docs/7_development/1_contributing.md](docs/7_development/1_contributing.md) *(coming soon)*

---

## License

**Business Source License 1.1** — Free for non-production use.  
Converts to **Apache 2.0** on **October 4, 2029**.

See [LICENSE](LICENSE) for full terms.

---

<p align="center">
  <strong>Volant</strong> — <em>Fast, lean, isolated. The way services should run.</em>
</p>
