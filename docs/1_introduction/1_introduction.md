 
## **Meet Volant**

**Volant** is a modular microVM orchestration engine — a platform that makes virtualization programmable.
It ships a compact control plane, CLI, and in-guest agent that share a common manifest format. Each workload runs inside its own microVM with deterministic networking, real resource isolation, and clean lifecycle control.

Runtime behavior lives in signed plugin manifests and their artifacts — kernels, initramfs bundles, or rootfs disks — keeping the core minimal while letting plugin authors define their environment precisely.

Modern infrastructure has drifted into abstraction overload.
Layers on layers have made simple things complex. **Volant moves the other way** — treating microVMs as first-class runtimes, combining the clarity and security of hardware virtualization with the ergonomics of containers.
No sidecars. No service meshes. No hidden daemons. Just a single binary and a clear contract between control plane and guest.

Volant exists to make infrastructure **predictable again.**
It unifies two worlds — **compatibility** and **performance** — under one control plane:

- **Run existing OCI images** as isolated microVMs for frictionless migration.
- **Build initramfs-based appliances** for performance-critical workloads.

Both paths share the same manifest, APIs, and networking fabric.
Volant replaces orchestration sprawl with a single, composable system that does one thing well:
**turn workloads into deterministic, hardware-isolated machines.**

---

## **A Search for Sanity**

Something is off with how we run software today.
We don’t hate containers — they’re indispensable — but somewhere along the way, we lost the plot.

A simple web server shouldn’t weigh **197 MB**. It shouldn’t drag in an entire userland, package manager, and libraries it never touches. Yet that’s become our definition of “lightweight.”

A decade of comfort left us with runtimes that are **bloated, opaque, and fragile** — systems so heavy with tooling that debugging the tooling takes longer than building the software itself.

Namespace “isolation” still shares a kernel.
Service meshes add layers to fix layers.
And deploying a basic service now requires consensus algorithms.

**Volant restores sanity** — stripping away unnecessary complexity and returning to a model that is secure by hardware design, transparent by construction, and predictable by default.

---

## **Bridging The Gap**

Volant isn’t another orchestration engine — it’s a **bridge between what’s familiar and what’s better.**

### For Universal Compatibility: The Rootfs Path

**Run any unmodified Docker/OCI image as a secure, high-performance microVM.**

Thousands of existing containerized applications can run unmodified inside Cloud Hypervisor microVMs with full hardware isolation and deterministic networking — **no code changes required.**

This is the **pragmatic path** — for when you need compatibility and migration ease.

### For Maximum Performance: The Initramfs Path

**Create hyper-optimized, appliance-style microVMs that boot in milliseconds.**

Package your static binary and dependencies into a minimal initramfs — no extra files, no unused packages, no wasted bytes.

- Boot times under 100 ms
- Memory footprints under 20 MB
- Attack surface measured in kilobytes
- Perfect for serverless-style or high-density workloads

This is the **performance path** — for when you need speed, efficiency, and total control.

**Both paths share the same platform, API, and tooling.**

---

## **What Makes Volant Different**

### Developer-First Design
Infrastructure isn’t inherently complex — the experience built around it is. Volant fixes that with a human-first design and frictionless workflows.

### True Hardware Isolation
Every workload runs in its own Cloud Hypervisor microVM — not a container, not a namespace — a real VM with its own kernel, isolated from the host at the CPU level. Security by design, not by sandbox trickery.

### Static, Predictable Networking
Each microVM gets a **static IP** from a deterministic pool. No overlays, no discovery layers — simple, reliable, and debuggable.

### The Dual-Kernel Strategy
Volant supports both compatibility and performance through two kernels:
1. **`bzImage-volant`** — Boots OCI/rootfs-based workloads.
2. **`vmlinux-generic`** — Boots initramfs appliances.

### Kestrel: The Intelligent Supervisor
Every microVM runs **kestrel**, Volant’s in-guest PID 1 that handles mounts, pivots, supervision, and manifest-driven orchestration. It’s the heartbeat of every VM.

---

## **The Core Components**

### volantd — The Control Plane
A single Go binary that manages state (SQLite), allocates IPs, orchestrates microVMs, hosts the plugin registry, and exposes REST + MCP APIs.
No dependencies. No consensus systems. Just one daemon.

### volar — The CLI
A scriptable tool that creates, lists, stops, and manages microVMs and plugins. Designed for both automation and direct use.

### kestrel — The In-Guest Agent
Handles two-stage boot, mounts essential filesystems, supervises workloads, performs health checks, and exposes an optional HTTP proxy.

### fledge — The Plugin Builder
Builds rootfs- or initramfs-based plugins from declarative configs. Reproducible, CI/CD-friendly, and minimal by default.

---

## **The Plugin Ecosystem**

Volant is **plugin-first.**
The core engine is generic; runtime-specific logic lives in manifests that define resources, entrypoints, and artifacts.
Manifests are validated, stored in SQLite, and injected into VMs at boot — self-contained, reproducible, and portable.

---

## **Real-World Use Cases**

1. **Isolated Browser Automation** — Hardware-level sandboxing for headless browsers.
2. **AI/ML Inference** — Secure, snapshot-ready GPU workloads.
3. **Multi-Tenant Dev Environments** — True isolation without Docker-in-Docker.
4. **Secure CI/CD Runners** — Disposable build VMs.
5. **Edge Nodes** — Lightweight, deterministic execution at the edge.
6. **Protocol Bridges** — Dedicated VMs for networking and gateway tasks.

---

## **Who Should Use Volant**

Engineers who demand **control, performance, and security** — from platform teams to AI infra engineers, plugin authors, and anyone tired of container bloat.

---

## **What Volant Is Not**

- **Not Kubernetes** — Focused on single-node or small-cluster orchestration.
- **Not a Container Runtime** — Runs VMs, not namespaces.
- **Not a Cloud Platform** — You own the data and deployment.
- **Not Magic** — Prioritizes simplicity and determinism over extreme density.

---

## **Core Principles**

1. **Hardware Isolation as a Primitive** — Every workload gets its own kernel.
2. **Simplicity Over Cleverness** — Static IPs, SQLite, and direct config over abstractions.
3. **Plugin-First Architecture** — Extensible without bloat.
4. **Developer Experience Matters** — Good defaults, clear logs, predictable tools.
5. **Security Without Compromise** — Reproducible builds, verified artifacts, minimal surface.

---

## **Performance Characteristics**

| Mode | Boot Time | Memory | Disk | Use Case |
|------|------------|--------|-------|-----------|
| **Rootfs (OCI)** | 2–5 s | ~50 MB + workload | Variable | Compatibility |
| **Initramfs** | 50–150 ms | 10–20 MB + workload | 5–50 MB | Performance |

Both paths support snapshot/restore and deterministic resource allocation.

---

## **Technology Stack**

**Hypervisor:** Cloud Hypervisor (Rust)
**Control Plane:** Go 1.22+
**Database:** SQLite
**Agent:** Go (static binary)
**C Shim:** Minimal init < 10 KB
**Networking:** Linux bridge + static IPAM
**Build Tools:** fledge, skopeo, umoci, busybox

No external dependencies beyond Linux + KVM.

---

## **Get Started**

1. **[Installation](2_getting-started/1_installation.md)** — Install in under 60 seconds.
2. **[Quick Start: Rootfs](2_getting-started/2_quick-start-rootfs.md)** — Run your first OCI image.
3. **[Quick Start: Initramfs](2_getting-started/3_quick-start-initramfs.md)** — Build and deploy an appliance.

**For Plugin Authors:**
- **[Plugin Development Intro](4_plugin-development/1_introduction.md)**
- **[Rootfs Guide](4_plugin-development/2_authoring-guide-rootfs.md)**
- **[Initramfs Guide](4_plugin-development/3_authoring-guide-initramfs.md)**

**For Deep Divers:**
- **[Architecture Overview](5_architecture/1_overview.md)**
- **[Boot Process](5_architecture/2_boot-process.md)**
- **[Control Plane Internals](5_architecture/3_control-plane.md)**

---

## **Community and Support**

Volant is open source under **Business Source License 1.1**, converting to **Apache 2.0** on Oct 4, 2029.

- **GitHub:** [github.com/volantvm/volant](https://github.com/volantvm/volant)
- **Docs:** [docs.volantvm.com](https://docs.volantvm.com)
- **Issues:** [github.com/volantvm/volant/issues](https://github.com/volantvm/volant/issues)
- **Discussions:** [github.com/volantvm/volant/discussions](https://github.com/volantvm/volant/discussions)

We ship fast. Volant already supports dual-kernel boot, OCI & initramfs paths, static IP management, plugin registry, REST/MCP APIs, deployments, and event streams.

**Coming Soon (1–3 months):** GPU passthrough, snapshots, and multi-host clustering.
**Coming Later (3–6 months):** integrated PaaS, snapshot-warmed serverless, advanced networking.

---

## **The Bottom Line**

Volant is **microVM orchestration done right** — simple, secure, and production‑ready.
Two paths. One platform. Real isolation. Predictable performance. Plugin‑first design.

**Build the runtime you need, without rebuilding the control plane.**

---

*Volant — The Intelligent Execution Cloud*

## **Meet Volant**

**Volant** is a modular microVM orchestration engine — a platform that makes virtualization programmable.
It ships a compact control plane, CLI, and in-guest agent that share a common manifest format. Each workload runs inside its own microVM with deterministic networking, real resource isolation, and clean lifecycle control.

Runtime behavior lives in signed plugin manifests and their artifacts — kernels, initramfs bundles, or rootfs disks — keeping the core minimal while letting plugin authors define their environment precisely.

Modern infrastructure has drifted into abstraction overload.
Layers on layers have made simple things complex. **Volant moves the other way** — treating microVMs as first-class runtimes, combining the clarity and security of hardware virtualization with the ergonomics of containers.
No sidecars. No service meshes. No hidden daemons. Just a single binary and a clear contract between control plane and guest.

Volant exists to make infrastructure **predictable again.**
It unifies two worlds — **compatibility** and **performance** — under one control plane:

- **Run existing OCI images** as isolated microVMs for frictionless migration.
- **Build initramfs-based appliances** for performance-critical workloads.

Both paths share the same manifest, APIs, and networking fabric.
Volant replaces orchestration sprawl with a single, composable system that does one thing well:
**turn workloads into deterministic, hardware-isolated machines.**

---

## **A Search for Sanity**

Something is off with how we run software today.
We don’t hate containers — they’re indispensable — but somewhere along the way, we lost the plot.

A simple web server shouldn’t weigh **197 MB**. It shouldn’t drag in an entire userland, package manager, and libraries it never touches. Yet that’s become our definition of “lightweight.”

A decade of comfort left us with runtimes that are **bloated, opaque, and fragile** — systems so heavy with tooling that debugging the tooling takes longer than building the software itself.

Namespace “isolation” still shares a kernel.
Service meshes add layers to fix layers.
And deploying a basic service now requires consensus algorithms.

**Volant restores sanity** — stripping away unnecessary complexity and returning to a model that is secure by hardware design, transparent by construction, and predictable by default.

---

## **Bridging The Gap**

Volant isn’t another orchestration engine — it’s a **bridge between what’s familiar and what’s better.**

### For Universal Compatibility: The Rootfs Path

**Run any unmodified Docker/OCI image as a secure, high-performance microVM.**

Thousands of existing containerized applications can run unmodified inside Cloud Hypervisor microVMs with full hardware isolation and deterministic networking — **no code changes required.**

This is the **pragmatic path** — for when you need compatibility and migration ease.

### For Maximum Performance: The Initramfs Path

**Create hyper-optimized, appliance-style microVMs that boot in milliseconds.**

Package your static binary and dependencies into a minimal initramfs — no extra files, no unused packages, no wasted bytes.

- Boot times under 100 ms
- Memory footprints under 20 MB
- Attack surface measured in kilobytes
- Perfect for serverless-style or high-density workloads

This is the **performance path** — for when you need speed, efficiency, and total control.

**Both paths share the same platform, API, and tooling.**

---

## **What Makes Volant Different**

### Developer-First Design
Infrastructure isn’t inherently complex — the experience built around it is. Volant fixes that with a human-first design and frictionless workflows.

### True Hardware Isolation
Every workload runs in its own Cloud Hypervisor microVM — not a container, not a namespace — a real VM with its own kernel, isolated from the host at the CPU level. Security by design, not by sandbox trickery.

### Static, Predictable Networking
Each microVM gets a **static IP** from a deterministic pool. No overlays, no discovery layers — simple, reliable, and debuggable.

### The Dual-Kernel Strategy
Volant supports both compatibility and performance through two kernels:
1. **`bzImage-volant`** — Boots OCI/rootfs-based workloads.
2. **`vmlinux-generic`** — Boots initramfs appliances.

### Kestrel: The Intelligent Supervisor
Every microVM runs **kestrel**, Volant’s in-guest PID 1 that handles mounts, pivots, supervision, and manifest-driven orchestration. It’s the heartbeat of every VM.

---

## **The Core Components**

### volantd — The Control Plane
A single Go binary that manages state (SQLite), allocates IPs, orchestrates microVMs, hosts the plugin registry, and exposes REST + MCP APIs.
No dependencies. No consensus systems. Just one daemon.

### volar — The CLI
A scriptable tool that creates, lists, stops, and manages microVMs and plugins. Designed for both automation and direct use.

### kestrel — The In-Guest Agent
Handles two-stage boot, mounts essential filesystems, supervises workloads, performs health checks, and exposes an optional HTTP proxy.

### fledge — The Plugin Builder
Builds rootfs- or initramfs-based plugins from declarative configs. Reproducible, CI/CD-friendly, and minimal by default.

---

## **The Plugin Ecosystem**

Volant is **plugin-first.**
The core engine is generic; runtime-specific logic lives in manifests that define resources, entrypoints, and artifacts.
Manifests are validated, stored in SQLite, and injected into VMs at boot — self-contained, reproducible, and portable.

---

## **Real-World Use Cases**

1. **Isolated Browser Automation** — Hardware-level sandboxing for headless browsers.
2. **AI/ML Inference** — Secure, snapshot-ready GPU workloads.
3. **Multi-Tenant Dev Environments** — True isolation without Docker-in-Docker.
4. **Secure CI/CD Runners** — Disposable build VMs.
5. **Edge Nodes** — Lightweight, deterministic execution at the edge.
6. **Protocol Bridges** — Dedicated VMs for networking and gateway tasks.

---

## **Who Should Use Volant**

Engineers who demand **control, performance, and security** — from platform teams to AI infra engineers, plugin authors, and anyone tired of container bloat.

---

## **What Volant Is Not**

- **Not Kubernetes** — Focused on single-node or small-cluster orchestration.
- **Not a Container Runtime** — Runs VMs, not namespaces.
- **Not a Cloud Platform** — You own the data and deployment.
- **Not Magic** — Prioritizes simplicity and determinism over extreme density.

---

## **Core Principles**

1. **Hardware Isolation as a Primitive** — Every workload gets its own kernel.
2. **Simplicity Over Cleverness** — Static IPs, SQLite, and direct config over abstractions.
3. **Plugin-First Architecture** — Extensible without bloat.
4. **Developer Experience Matters** — Good defaults, clear logs, predictable tools.
5. **Security Without Compromise** — Reproducible builds, verified artifacts, minimal surface.

---

## **Performance Characteristics**

| Mode | Boot Time | Memory | Disk | Use Case |
|------|------------|--------|-------|-----------|
| **Rootfs (OCI)** | 2–5 s | ~50 MB + workload | Variable | Compatibility |
| **Initramfs** | 50–150 ms | 10–20 MB + workload | 5–50 MB | Performance |

Both paths support snapshot/restore and deterministic resource allocation.

---

## **Technology Stack**

**Hypervisor:** Cloud Hypervisor (Rust)
**Control Plane:** Go 1.22+
**Database:** SQLite
**Agent:** Go (static binary)
**C Shim:** Minimal init < 10 KB
**Networking:** Linux bridge + static IPAM
**Build Tools:** fledge, skopeo, umoci, busybox

No external dependencies beyond Linux + KVM.

---

## **Get Started**

1. **[Installation](2_getting-started/1_installation.md)** — Install in under 60 seconds.
2. **[Quick Start: Rootfs](2_getting-started/2_quick-start-rootfs.md)** — Run your first OCI image.
3. **[Quick Start: Initramfs](2_getting-started/3_quick-start-initramfs.md)** — Build and deploy an appliance.

**For Plugin Authors:**
- **[Plugin Development Overview](../4_plugin-development/1_overview.md)**
- **[Initramfs Guide](../4_plugin-development/2_initramfs.md)**
- **[OCI Rootfs Guide](../4_plugin-development/3_oci-rootfs.md)**

**For Deep Divers:**
- **[Architecture Overview](../5_architecture/1_overview.md)**

---

## **Community and Support**

Volant is open source under **Business Source License 1.1**, converting to **Apache 2.0** on Oct 4, 2029.

- **GitHub:** [github.com/volantvm/volant](https://github.com/volantvm/volant)
- **Docs:** [docs.volantvm.com](https://docs.volantvm.com)
- **Issues:** [github.com/volantvm/volant/issues](https://github.com/volantvm/volant/issues)
- **Discussions:** [github.com/volantvm/volant/discussions](https://github.com/volantvm/volant/discussions)

We ship fast. Volant already supports dual-kernel boot, OCI & initramfs paths, static IP management, plugin registry, REST/MCP APIs, deployments, and event streams.

**Coming Soon (1–3 months):** GPU passthrough, snapshots, and multi-host clustering.
**Coming Later (3–6 months):** integrated PaaS, snapshot-warmed serverless, advanced networking.

---

## **The Bottom Line**

Volant is **microVM orchestration done right** — simple, secure, and production‑ready.
Two paths. One platform. Real isolation. Predictable performance. Plugin‑first design.

**Build the runtime you need, without rebuilding the control plane.**

---

*Volant — The Intelligent Execution Cloud*
