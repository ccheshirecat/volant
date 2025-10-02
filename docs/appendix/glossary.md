---
title: "Glossary"
description: "Terminology used throughout the Volant documentation."
---

# Glossary

- **Volant** – MicroVM orchestration engine focused on secure, plugin-driven runtimes.
- **volantd** – Go binary providing REST/MCP APIs and managing Cloud Hypervisor VMs.
- **volary** – In-VM Go agent that hydrates plugin-defined workloads and exposes HTTP/WebSocket routes.
- **volar CLI** – Cobra command-line tooling for scripts.
- **TUI** – Previously an interactive Bubble Tea dashboard; removed to focus on core orchestration.
- **Cloud Hypervisor** – Lightweight VMM used for launching microVMs.
- **Initramfs** – Embedded into the Linux kernel bzImage; built via `build/bake.sh` and compiled into the kernel with `CONFIG_INITRAMFS_SOURCE`.
- **Kernel** – Linux kernel image (`bzImage`) from the Cloud Hypervisor fork with embedded initramfs.
- **Bridge (`vbr0`)** – Host Linux bridge providing static IP networking for microVMs.
- **MCP** – Model Context Protocol endpoint for LLM-driven automation (`POST /api/v1/mcp`).
- **AG-UI** – Legacy Agent-UI WebSocket event stream (removed).
- **SSE** – Server-Sent Events (`/api/v1/events/vms`).
- **DevTools Proxy** – WebSocket forwarding of Chrome DevTools (`/ws/v1/vms/{name}/devtools`).
- **Artifacts** – Kernel available at `kernels/<arch>/bzImage` per release; initramfs archive produced by `build/bake.sh` for kernel integration.
- **Installer** – `scripts/install.sh` bootstrapper for host setup.
- **Runtime dir** – `~/.volant/run`, default socket/log location per user.
- **Event bus** – Internal pub/sub delivering VM lifecycle events to multiple subscribers.
