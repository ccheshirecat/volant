# Introduction to Volant

## The Problem: The 197MB NGINX Container

Something is deeply wrong with modern infrastructure.

A simple web server—a single static binary that serves HTTP requests—should not require 197MB of container image bloat. It should not need an entire Ubuntu userland, a package manager, and dozens of libraries it will never use. Yet this is the reality we've accepted as "normal" in the container era.

The current state of containerization is **bloated, insecure, and inefficient**. Docker images bundle entire operating systems for single-purpose applications. Container "isolation" relies on kernel namespaces and cgroups—sophisticated sandboxes, but still fundamentally shared-kernel environments vulnerable to escape exploits. And orchestration platforms have become so complex that deploying a simple service requires understanding overlay networks, service meshes, and distributed consensus algorithms.

**We built Volant to fix this.**

---

## The Volant Promise: Two Worlds, One Platform

Volant is not just another orchestration engine. It's a **bridge between two fundamentally different approaches** to running workloads, unified under a single control plane:

### For Universal Compatibility: The Rootfs Path

**Run any unmodified Docker/OCI image as a secure, high-performance microVM.**

You have existing containerized applications. Thousands of images in your registry. Years of Dockerfiles and build pipelines. Volant provides an easy, "it just works" path to take those applications and give them **true hardware-level isolation** without rewriting anything.

- Take any OCI image from Docker Hub, GHCR, or your private registry
- Boot it in a Cloud Hypervisor microVM with a real Linux kernel
- Get hardware isolation, dedicated networking, and deterministic resource allocation
- Zero application changes required

This is the **pragmatic path**—for when you need compatibility and migration ease.

### For Maximum Performance: The Initramfs Path

**Create hyper-optimized, appliance-style microVMs that boot in milliseconds.**

For those who demand absolute peak performance, Volant offers a "golden path" to build purpose-built microVMs. Package your static binary, your application, your exact dependencies into a tiny initramfs. No extra files. No unused packages. No attack surface beyond what you explicitly include.

- Boot times under 100ms
- Memory footprints under 20MB
- Attack surface measured in kilobytes, not gigabytes
- Reproducible, deterministic builds
- Perfect for serverless-style workloads with snapshot/restore

This is the **performance path**—for when you need speed, efficiency, and absolute control.

**Both paths run on the same platform. Same control plane. Same API. Same tooling.**

---

## What Makes Volant Different

### True Hardware Isolation

Every workload runs in its own Cloud Hypervisor microVM. Not a container. Not a namespace. A real virtual machine with its own kernel, isolated from the host at the CPU instruction level. This is security by hardware design, not by kernel tricks.

### Static, Predictable Networking

No overlay networks. No complex service discovery. Each microVM gets a **static IP address** on a bridge network, allocated from a deterministic pool. Simple. Reliable. Debuggable.

When you create a VM, you know its IP address. When it dies, the IP returns to the pool. No surprises. No magic.

### The Dual-Kernel Strategy

This is one of Volant's key architectural innovations:

1. **`bzImage-volant`** — A kernel with a baked-in initramfs bootloader. Used for **rootfs-based plugins** (OCI images). The bootloader mounts your disk image, pivots into it, and boots your application.

2. **`vmlinux-generic`** — A pristine, blank-slate ELF kernel. Used for **initramfs-based plugins**. The initramfs (your custom-built appliance) is provided at runtime via the `--initramfs` flag.

This dual strategy allows Volant to support both high-compatibility (rootfs) and high-performance (initramfs) workloads with the same control plane.

### Kestrel: The Sophisticated Supervisor

Inside every microVM runs **kestrel**, Volant's intelligent PID 1 supervisor. Kestrel is not a simple init replacement—it's a two-stage boot coordinator:

- **Stage 1 (C shim)**: A minimal C program that sets up the filesystem hierarchy, mounts `/proc`, `/sys`, `/dev`, and hands off to kestrel
- **Stage 2 (kestrel as PID 1)**: Handles rootfs pivoting (if needed), mounts essential filesystems, starts your workload, and acts as a process supervisor and API proxy

Kestrel reads the plugin manifest from the kernel command line, understands your workload's requirements, and brings it to life. It's the soul of every Volant microVM.

---

## The Core Components

### volantd (The Control Plane)

The brain of Volant. A single Go binary that:

- **Manages state** using an embedded SQLite database (single source of truth)
- **Allocates IP addresses** from configured subnets with lease tracking
- **Orchestrates microVMs** with Cloud Hypervisor
- **Hosts the plugin registry** and enforces enablement policies
- **Exposes REST and MCP APIs** for external control
- **Provides event streaming** for observability and automation
- **Supports deployments** with declarative scaling (Kubernetes-style controllers)

No external dependencies. No distributed consensus. No configuration sprawl. Just one daemon and a SQLite file.

### volar (The CLI)

Your interface to Volant. A scriptable command-line tool that:

- Creates, lists, stops, and deletes microVMs
- Manages plugins (install, enable, disable, list)
- Configures the host (bridge networking, NAT, systemd service)
- Provides interactive shell access to running VMs
- Displays logs, events, and health status
- Integrates with CI/CD pipelines via simple commands

Clean, composable commands that work in scripts or interactively.

### kestrel (The In-Guest Agent)

The supervisor that runs as PID 1 inside each microVM:

- **Two-stage boot process**: C shim → kestrel
- **Rootfs pivot** (for OCI images): Mounts `/dev/vda`, copies itself, `switch_root` into the disk
- **Essential mounts**: `/proc`, `/sys`, `/dev`, `/tmp`, `/run`
- **Workload supervision**: Reads manifest, spawns your entrypoint with proper env/cwd, monitors process groups
- **HTTP API proxy**: Optionally forwards requests from the control plane to your workload
- **Health checks**: Validates workload readiness
- **Debug shell**: Optional serial console access for troubleshooting

Kestrel is battle-tested and handles the complex boot dance so your applications don't have to.

### fledge (The Plugin Builder)

The official toolkit for creating Volant plugins:

- **Declarative TOML configuration** (`fledge.toml`)
- **Two build strategies**: rootfs (from OCI images) or initramfs (from scratch)
- **Automatic agent sourcing**: Downloads kestrel from GitHub releases
- **File mapping system**: FHS-aware permissions, custom overlays
- **Reproducible builds**: Deterministic timestamps, checksums
- **CI/CD ready**: Perfect for GitHub Actions workflows

Fledge is the "compiler" for the Volant ecosystem. You describe what you want; fledge builds it.

---

## The Plugin Ecosystem

Volant is **plugin-first**. The core engine knows nothing about browsers, databases, or AI models. All runtime-specific logic lives in **plugins**, which are defined by manifests.

A plugin manifest is a JSON file that declares:

```json
{
  "schema_version": "1.0",
  "name": "caddy",
  "version": "0.1.0",
  "runtime": "caddy",
  "enabled": true,
  "initramfs": {
    "url": "/path/to/plugin.cpio.gz",
    "checksum": "sha256:abc123..."
  },
  "resources": {
    "cpu_cores": 1,
    "memory_mb": 512
  },
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/bin/caddy", "run", "--config", "/etc/caddy/Caddyfile"],
    "base_url": "http://127.0.0.1:80",
    "env": {
      "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    }
  },
  "health_check": {
    "endpoint": "/",
    "timeout_ms": 10000
  }
}
```

The control plane:
- Stores the manifest in SQLite
- Validates it against the schema
- Enforces enablement (disabled plugins can't be used)
- Injects it into the VM via kernel cmdline (base64-encoded, gzipped)

The agent (kestrel):
- Reads the manifest from `/proc/cmdline`
- Configures the environment
- Starts the workload
- Performs health checks

**Plugins are self-contained.** Each manifest references its own artifacts (initramfs, rootfs, additional disks). Plugin authors can distribute their work independently, and users install them with a single command.

---

## Real-World Use Cases

### 1. **Isolated Browser Automation**

Run headless Chrome/Firefox instances in hardware-isolated microVMs. Each session gets its own kernel, filesystem, and network stack. No shared state. No cross-contamination. Perfect for security-sensitive scraping or testing.

### 2. **AI/ML Inference Workloads**

Deploy ML models in microVMs with GPU/VFIO passthrough (coming soon). Isolation prevents model theft or data leakage. Snapshot/restore enables instant cold-start for serverless-style inference.

### 3. **Multi-Tenant Development Environments**

Give each developer their own isolated VM with their preferred tools pre-installed. No containers fighting over ports. No Docker-in-Docker hacks. Just real VMs.

### 4. **Secure CI/CD Runners**

Run untrusted build jobs in disposable microVMs. Full isolation. Clean slate every time. No risk of escape exploits compromising your build infrastructure.

### 5. **Edge Computing Nodes**

Deploy lightweight microVMs on edge devices. Minimal resource overhead. Fast boot times. Deterministic behavior. Perfect for IoT gateways and edge analytics.

### 6. **Protocol Bridges and Proxies**

Run specialized network services (VPN endpoints, protocol translators, API gateways) in isolated VMs. Each service gets its own dedicated network interface.

---

## Who Should Use Volant?

Volant is built for **engineers who demand control, performance, and security**:

- **Platform engineers** building internal developer platforms or PaaS offerings
- **Security teams** requiring hardware-level isolation for sensitive workloads
- **AI infrastructure engineers** orchestrating GPU workloads with deterministic resource allocation
- **Plugin authors** productizing specialized runtimes (browsers, databases, custom interpreters)
- **Anyone tired of container bloat** who wants a leaner, faster, more secure alternative

If you've ever thought "this container is way too big for what it does" or "I wish I had real isolation without Kubernetes complexity," Volant is for you.

---

## What Volant Is Not

Let's be clear about what Volant doesn't try to be:

- **Not a Kubernetes replacement**: Volant is focused on single-node or small-cluster orchestration. No distributed control plane. No etcd. If you need multi-datacenter pod scheduling, use Kubernetes.

- **Not a container runtime**: Volant runs microVMs, not containers. VMs boot slower than containers (though initramfs boots are under 100ms), but you get real hardware isolation.

- **Not a cloud platform**: Volant is infrastructure software you run yourself. No SaaS. No vendor lock-in. You control the data and the deployment.

- **Not magic**: Volant makes intelligent tradeoffs. It prioritizes simplicity, determinism, and security over absolute maximum density. You won't pack 1000 VMs on a single host like you would with containers.

---

## Core Principles

### 1. Hardware Isolation as a Primitive

Namespaces and cgroups are useful, but they're not real isolation. Volant embraces hardware virtualization as the foundation. Every workload gets its own kernel. This is security by design.

### 2. Simplicity Over Cleverness

SQLite instead of distributed databases. Static IP allocation instead of overlay networks. Kernel cmdline for configuration instead of complex service discovery. Simple, debuggable, predictable.

### 3. Plugin-First Architecture

The core engine stays lean. Domain-specific logic lives in plugins. This keeps Volant maintainable and extensible without becoming a monolith.

### 4. Developer Experience Matters

Good defaults. Clear error messages. Readable logs. Scriptable CLI. No magic incantations. Tools should work the way you expect.

### 5. Security Without Compromise

Hardware isolation. Reproducible builds. Checksum verification. Minimal attack surface. Security is not an afterthought; it's the foundation.

---

## Performance Characteristics

### Rootfs Path (OCI Images)
- **Boot time**: 2-5 seconds (cold start with disk mount and pivot)
- **Memory overhead**: ~50MB base + workload
- **Disk**: Variable (depends on OCI image size)
- **Use case**: Compatibility, quick migration from Docker

### Initramfs Path (Custom Appliances)
- **Boot time**: 50-150ms (from power-on to workload start)
- **Memory overhead**: 10-20MB base + workload
- **Disk**: 5-50MB (typical appliance size)
- **Use case**: Performance, serverless, high-density deployments

Both paths benefit from:
- **Zero network setup time** (static IPs pre-assigned)
- **Snapshot/restore** for instant resume
- **Deterministic resource allocation** (dedicated CPU cores, fixed memory)

---

## The Technology Stack

**Hypervisor**: Cloud Hypervisor (KVM-based, written in Rust)
**Control Plane**: Go 1.22+
**Database**: SQLite (embedded, no external services)
**Agent**: Go (compiled to static binary, included in initramfs)
**C Shim**: Minimal init (compiled with gcc, <10KB)
**Networking**: Linux bridge + static IPAM
**Build Tools**: fledge (Go), skopeo, umoci, busybox

No external dependencies beyond Linux, KVM, and standard system tools.

---

## Get Started

Enough philosophy. Let's build something.

**Next Steps:**

1. **[Installation](2_getting-started/1_installation.md)** — Install Volant in under 60 seconds
2. **[Quick Start: Rootfs](2_getting-started/2_quick-start-rootfs.md)** — Run your first OCI image as a microVM
3. **[Quick Start: Initramfs](2_getting-started/3_quick-start-initramfs.md)** — Build and deploy a hyper-optimized appliance

**For Plugin Authors:**

- **[Plugin Development Introduction](4_plugin-development/1_introduction.md)** — Understand the two build strategies
- **[Authoring Guide: Rootfs](4_plugin-development/2_authoring-guide-rootfs.md)** — Convert OCI images to Volant plugins
- **[Authoring Guide: Initramfs](4_plugin-development/3_authoring-guide-initramfs.md)** — Build custom appliances with fledge

**For Deep Divers:**

- **[Architecture Overview](5_architecture/1_overview.md)** — System components and data flow
- **[Boot Process](5_architecture/2_boot-process.md)** — The deep magic of dual kernels and two-stage boot
- **[Control Plane Internals](5_architecture/3_control-plane.md)** — How volantd works under the hood

---

## Community and Support

Volant is open source under the **Business Source License 1.1**, which converts to **Apache License 2.0** on October 4, 2029.

- **GitHub**: [github.com/volantvm/volant](https://github.com/volantvm/volant)
- **Documentation**: [docs.volantvm.com](https://docs.volantvm.com)
- **Issues**: [github.com/volantvm/volant/issues](https://github.com/volantvm/volant/issues)
- **Discussions**: [github.com/volantvm/volant/discussions](https://github.com/volantvm/volant/discussions)

We ship fast. Volant is less than a month old and already supports:
- Dual-kernel boot strategies
- OCI image compatibility
- Custom initramfs appliances
- Static IP management
- Plugin registry and manifests
- REST and MCP APIs
- Deployment orchestration
- Event streaming

**Coming Soon** (next 1-3 months):
- VFIO GPU passthrough for AI/ML workloads
- Snapshot/restore for instant cold-starts
- Multi-host clustering (simple, not distributed consensus)

**Coming Later** (3-6 months):
- Integrated PaaS platform (think Vercel/Dokploy but for any workload)
- Serverless-style function execution with snapshot warm-up
- Advanced networking (multiple bridges, VLANs, policy routing)

---

## The Bottom Line

Volant is **microVM orchestration done right**.

- **Two paths** (rootfs for compatibility, initramfs for performance)
- **One platform** (unified control plane, API, tooling)
- **Real isolation** (hardware virtualization, not kernel tricks)
- **Simple architecture** (SQLite, static IPs, no magic)
- **Plugin-first** (extensible without bloat)
- **Production-ready** (battle-tested components, comprehensive docs)

Stop accepting bloated containers and shared-kernel sandboxes as the only option.

**Build the runtime you need, without rebuilding the control plane.**

---

*Volant — The Intelligent Execution Cloud*
