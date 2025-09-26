---
title: Introduction
description: Meet Volant, the modular microVM orchestrator.
---

# Volant

Volant is a **modular microVM orchestration engine**. It gives you a production-ready control plane, CLI, and automation surface for running secure, stateful workloads inside Cloud Hypervisor microVMs.

The engine ships with opinionated defaults so you can go from zero to isolated workloads in minutes, while staying flexible enough to power bespoke runtime plugins (browser automation, AI inference, workers, and more).

---

## Why a microVM orchestrator?

The industry standard for orchestration is still container sandboxes. They’re fast, but they were never designed for long-lived, stateful, or security-sensitive automation. Volant takes a different path:

| Capability          | Containers                            | Volant MicroVMs                               |
| ------------------- | ------------------------------------- | ---------------------------------------------- |
| Isolation           | Namespace / cgroup sandbox            | **Hardware-virtualized microVM**               |
| Networking          | Overlay-centric                       | **Explicit bridge with static IP leasing**     |
| State persistence   | Volumes, manual copy                  | **Snapshot-first architecture**                |
| Runtime plugability | Images baked into orchestrator        | **Declarative manifest per runtime**           |

We package those primitives behind a simple control plane (`volantd`), CLI/TUI (`volar`), and agent (`volary`) so teams can focus on what they run—not how to glue together virtualization plumbing.

---

## What’s in the box?

- **Control plane (`volantd`)**: native microVM orchestrator, IPAM, event bus, plugin registry, REST/MCP/AG-UI APIs.
- **CLI & TUI (`volar`)**: scriptable commands and a terminal dashboard for operators.
- **Agent (`volary`)**: runtime host inside the microVM that exposes plugin-defined actions over HTTP/WebSocket.
- **Plugin system**: manifests describe runtimes, resources, and action proxies—so specialized workloads can live in dedicated repos.

---

## Who uses Volant?

- Platform teams provisioning secure, isolated execution environments for internal automation.
- AI infrastructure engineers needing stateful, observable runtimes for model agents.
- Security researchers who want deterministic, disposable environments.
- Plugin authors building reusable runtime bundles (browser automation, HTTP clients, scraping, RPA, etc.).

---

## Design principles

1. **Batteries-included, buildable-out**: A single binary gets you networking, orchestration, and APIs, but every layer is pluggable.
2. **MicroVM-first**: All workload isolation is hardware-virtualized. The orchestrator speaks Cloud Hypervisor directly.
3. **Declarative runtimes**: Plugins describe resources and actions in manifests; the engine handles lifecycle.
4. **Plain dependencies**: SQLite for state, Go binaries for daemon/CLI/agent, Docker-based image tooling for initramfs builds.

---

## Next steps

- [Installation](installation.md): bootstrap the engine on a host.
- [Control Plane Overview](../guides/control-plane.md): deep dive into `volantd` internals.
- [CLI & TUI](../guides/cli-and-tui.md): operating the engine.
- [Plugin Guide](../guides/plugins.md): how manifests and runtimes fit together.

For detailed APIs, see the [REST](../api/rest-api.md) and [MCP](../api/mcp.md) references.