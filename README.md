<p align="center">
  <img src="banner.png" alt="VOLANT — The Intelligent Execution Cloud"/>
</p>

<p align="center">
  <a href="https://github.com/ccheshirecat/volant/actions">
    <img src="https://img.shields.io/github/actions/workflow/status/ccheshirecat/volant/ci.yml?branch=main&style=flat-square&label=tests" alt="Build Status">
  </a>
  <a href="https://github.com/ccheshirecat/volant/releases">
    <img src="https://img.shields.io/github/v/release/ccheshirecat/volant.svg?style=flat-square" alt="Latest Release">
  </a>
  <a href="https://golang.org/">
    <img src="https://img.shields.io/badge/Go-1.22+-black.svg?style=flat-square" alt="Go Version">
  </a>
  <a href="https://github.com/ccheshirecat/volant/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/License-Apache_2.0-black.svg?style=flat-square" alt="License">
  </a>
</p>

---

# Volant

> **The modular microVM orchestration engine.**

Volant turns microVMs into a first-class runtime surface. The project ships a control plane, CLI/TUI, and agent that speak a common plugin contract so teams can run secure, stateful workloads without stitching together networking, scheduling, and lifecycle plumbing themselves.

Runtime-specific behavior lives in plugins. The core engine stays lean, plugin authors ship their own kernels/initramfs overlays, and operators choose what to enable.

---

## Overview

- **Control plane (`volantd`)** manages SQLite-backed state, static IP leasing, orchestration, REST/MCP/AG-UI APIs, and the plugin registry.
- **Agent (`volary`)** boots inside each microVM, hydrates the declared runtime, and mounts plugin-defined HTTP/WebSocket routes.
- **CLI & TUI (`volar`)** provide a dual-mode operator experience: scriptable Cobra commands and a Bubble Tea dashboard.
- **Plugins** declare resources, actions, and health probes via manifests—letting browser automation, AI inference, worker pools, or custom stacks share the same engine.

---

## Highlights

- 🛡 **Hardware isolation first** – every workload runs inside a Cloud Hypervisor microVM with static network bridging.
- 🧩 **Plugin contract** – manifests capture runtime requirements, action endpoints, and OpenAPI metadata.
- 🔌 **Universal proxy** – the control plane can forward REST, SSE, or WebSocket traffic to runtime agents without exposing private IPs.
- 📡 **AI-native APIs** – REST, Model Context Protocol, and AG-UI event streams ship in the box.
- 🧰 **Operator ergonomics** – one binary installs networking, bootstraps the database, and exposes both CLI and TUI surfaces.

---

## Quick start

```bash
# Install the Volant toolchain (binaries, kernel, initramfs)
curl -sSL https://install.volant.cloud | bash

# Configure the host (bridge networking, NAT, systemd service)
sudo volar setup

# Create a microVM using the default runtime manifest
volar vms create demo --cpu 2 --memory 2048

# Install a plugin (example: browser runtime defined in a separate repo)
volar plugins install browser --manifest ./manifests/browser.json

# Invoke a plugin action against the VM
volar vms action demo browser navigate --payload '{"url":"https://example.com"}'
```

Refer to `docs/guides/plugins.md` for manifest structure and distribution workflows.

---

## Architecture at a glance

| Layer | Responsibility |
| ----- | -------------- |
| Control plane | Persist state (SQLite), lease IPs, spawn microVMs, proxy agent traffic, emit events |
| Agent | Boot runtime, expose plugin routes, stream logs, surface DevTools info when available |
| Plugins | Provide kernels/initramfs overlays, declare resources/actions, publish OpenAPI metadata |
| Tooling | `volar` CLI/TUI + REST/MCP clients consuming the same manifests |

---

## Plugin workflow

1. **Author** a manifest (`name`, `version`, `runtime`, resource envelope, actions with method/path).
2. **Package** kernel/initramfs bundles or additional artifacts referenced by the manifest.
3. **Install** via `volar plugins install <name> --manifest path/to/manifest.json`.
4. **Enable/disable** with `volar plugins enable <name>` or `volar plugins disable <name>`.
5. **Call actions** using VM-scoped or global endpoints:
   ```bash
   volar vms action <vm> <plugin> <action> --payload ./payload.json
   volar plugins action <plugin> <action> --payload ./payload.json
   ```

The engine persists manifests, enforces enablement state, and resolves action routing so microVMs only run compatible runtimes.

---

## Repository layout

```
cmd/               # Entry points (volantd, volar, volary)
internal/          # Control plane, agent runtime, CLI/TUI, protocols
  agent/
  cli/
  protocol/
  server/
  setup/
build/             # Kernel/initramfs tooling
docs/              # Product documentation
Makefile           # Build + setup automation
go.mod / go.sum
```

---

## Development

1. Install **Go 1.22+** and Docker.
2. Build binaries: `make build` (or `make volantd volar volary`).
3. Build artifacts (kernel/initramfs): `make build-images`.
4. Run integration SQLite migrations: `make migrate`.
5. Launch the control plane locally:
   ```bash
   ./bin/volantd --config ./configs/dev.yaml
   ./bin/volar vms list
   ```

See `docs/guides/development.md` for deeper instructions.

---

## Documentation

The latest guides live in [`docs/`](docs) and at [docs.volant.cloud](https://docs.volant.cloud) once published.

Key entry points:
- [Start here](docs/start/introduction.md)
- [Plugin authoring](docs/guides/plugins.md)
- [REST API](docs/api/rest-api.md)
- [MCP interface](docs/api/mcp.md)

---

## Contributing

Pull requests, issues, and plugin proposals are welcome. Please see `CONTRIBUTING.md` (coming soon) for workflow details.

---

## License

Apache 2.0 – see [LICENSE](LICENSE).

---

<p align="center"><i>Volant — Build the runtime you need, without rebuilding the control plane.</i></p>
