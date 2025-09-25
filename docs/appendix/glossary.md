---
title: "Glossary"
description: "Terminology used throughout the Volant documentation."
---

# Glossary

- **Volant** – MicroVM automation platform combining orchestration, headless Chrome, and automation APIs.
- **volantd** – Go binary providing REST/MCP/AG-UI APIs and managing Cloud Hypervisor VMs.
- **volary** – In-VM Go service proxying browser automation commands to chromedp/headless-shell.
- **volar CLI** – Cobra command-line tooling for scripts.
- **TUI** – Interactive Bubble Tea dashboard accessible by running `volar` with no args.
- **Cloud Hypervisor** – Lightweight VMM used for launching microVMs.
- **Initramfs** – Initramfs image (`volant-initramfs.cpio.gz`) that boots the agent.
- **Kernel** – Linux kernel image (`vmlinux-x86_64`) paired with initramfs.
- **Bridge (`vbr0`)** – Host Linux bridge providing static IP networking for microVMs.
- **MCP** – Model Context Protocol endpoint for LLM-driven automation (`POST /api/v1/mcp`).
- **AG-UI** – Agent-UI WebSocket event stream (`/ws/v1/agui`) for dashboards.
- **SSE** – Server-Sent Events (`/api/v1/events/vms`).
- **DevTools Proxy** – WebSocket forwarding of Chrome DevTools (`/ws/v1/vms/{name}/devtools`).
- **Artifacts** – Build outputs located at `build/artifacts/` (kernel, initramfs, checksums).
- **Installer** – `scripts/install.sh` bootstrapper for host setup.
- **Runtime dir** – `~/.volant/run`, default socket/log location per user.
- **Event bus** – Internal pub/sub delivering VM lifecycle events to multiple subscribers.
