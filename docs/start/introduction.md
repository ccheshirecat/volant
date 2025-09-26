---
title: Introduction
description: Meet Volant, the modular microVM orchestration engine.
---

# Volant

Volant is a **modular microVM orchestration engine**. It pairs a production-grade control plane with a runtime plugin system so teams can launch secure workloads inside Cloud Hypervisor microVMs without wiring together networking, scheduling, or lifecycle management by hand.

The engine delivers sane defaults out of the box while remaining intentionally neutral about what runs inside each VM. Browser automation, AI inference, protocol bridges, and bespoke workloads all live behind plugins that declare their needs and expose actions through Volant’s APIs.

---

## Why microVM orchestration?

Container sandboxes were built for short-lived stateless jobs. Long-running automation, security-sensitive agents, and sessionful tooling benefit from hardware-isolated environments with predictable networking. Volant embraces that constraint:

| Capability | Containers | Volant MicroVMs |
| ---------- | ---------- | ---------------- |
| Isolation | Namespace / cgroup sandbox | **Hardware-virtualized microVM** |
| Networking | Overlay abstraction | **Static bridge with explicit IP leasing** |
| State | Volume mounts, ad-hoc scripting | **Snapshot-first lifecycle, disk imaging hooks** |
| Runtime surface | Images baked into orchestrator | **Declarative manifests per plugin** |

---

## Core components

- **Control plane (`volantd`)** – manages scheduling, IPAM, eventing, SQLite-backed state, and the plugin registry. Exposes REST, MCP, and AG-UI interfaces.
- **Agent (`volary`)** – runs inside each microVM, boots the declared runtime, and mounts plugin-defined HTTP/WebSocket routes.
- **CLI & TUI (`volar`)** – a dual-mode operator interface: scriptable Cobra commands plus an interactive Bubble Tea dashboard.
- **Runtime plugins** – manifests describe required images, CPU/memory envelopes, health checks, and action endpoints. The engine treats every plugin uniformly.

---

## Plugin ecosystem

Volant ships with a minimal engine bundle. Runtime-specific capabilities are delivered as plugins that can live in independent repositories. A plugin manifest declares:

- Runtime identifier (e.g., `browser`, `python-worker`)
- Resource requirements and kernel/initramfs selections
- Action endpoints exposed by the agent
- Optional OpenAPI reference for downstream tooling

The control plane persists manifests, enforces enablement/disablement, and proxies action calls to the right microVM. Browser automation is the first reference plugin, but nothing in the engine assumes its presence.

---

## Who builds on Volant?

- Platform teams providing isolated automation environments to internal users
- AI infrastructure engineers orchestrating stateful agents or toolchains
- Security and research labs needing reproducible, inspectable microVMs
- Plugin authors productizing specialized runtimes (browser orchestration, headless APIs, robotics control, custom interpreters)

---

## Guiding principles

1. **Batteries-included, plugin-first** – the core binary manages microVM plumbing; domain logic lives in plugins.
2. **Hardware isolation as a primitive** – Cloud Hypervisor integration, static IP allocation, and deterministic boot flow.
3. **Declarative runtime contracts** – manifests capture resources, actions, and health semantics so operators, CLIs, and MCP clients speak the same language.
4. **Plain dependencies** – Go binaries, SQLite state, Docker-based build pipeline for kernels/initramfs; no external control-plane services.

---

## Next steps

- [Installation](installation.md) – bootstrap the engine and database.
- [Control Plane Overview](../guides/control-plane.md) – internal architecture of `volantd`.
- [CLI & TUI](../guides/cli-and-tui.md) – scriptable commands and interactive ops surface.
- [Plugin Guide](../guides/plugins.md) – authoring and distributing runtime manifests.

When you are ready to integrate with tooling, explore the [REST API](../api/rest-api.md) and [MCP interface](../api/mcp.md).