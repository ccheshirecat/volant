# OVERHYPED

Overhyped is a production-grade, single-node microVM browser automation platform. It fuses a native orchestrator, an opinionated VM image pipeline, and human/AI-first control surfaces into a cohesive "private browser cloud" experience.

## Current Status
- **Phase:** Foundations (repository bootstrapping, persistence layer online).
- **Orchestrator:** Cloud Hypervisor launcher wired with deterministic networking and crash monitoring (REST/API layers pending).
- **Persistence:** Embedded SQLite store with migrations and typed VM/IP repositories (unit-tested).
- **Agent/Image:** Build pipeline and chromedp integration not yet implemented.
- **CLI/TUI:** Cobra + Bubble Tea entry points stubbed for future expansion.

Refer to `docs/roadmap.md` and `docs/development-tracker.md` for detailed milestones and live status.

## Guiding Tenets
- Deterministic static networking and Cloud Hypervisor for microVM lifecycle.
- Embedded SQLite database as the single source of truth.
- Dual-mode CLI offering scriptable commands and a "God Mode" Bubble Tea TUI.
- Native support for MCP and AG-UI protocols to empower AI-driven workflows.

## Repository Layout
```
cmd/                # Entry points for server, CLI, and in-VM agent
internal/           # Shared implementation packages
  agent/            # Agent runtime and chromedp integration (stubbed)
  cli/              # Cobra CLI and Bubble Tea TUI layers
  server/           # Orchestrator, persistence, API, event bus
  shared/           # Cross-cutting concerns (logging, config helpers)
docs/               # Roadmap, development tracker, architecture notes
build/              # Image pipeline assets (Docker → initramfs)
scripts/            # Operational helpers and automation
```

## Getting Started (Development)
1. Ensure Go 1.22+ is installed locally.
2. Export the required environment variables before running `hyped`:
   - `OVERHYPED_KERNEL` – absolute path to the microVM kernel image.
   - `OVERHYPED_INITRAMFS` – absolute path to the initramfs bundle.
   - `OVERHYPED_HOST_IP` – host-side IP inside the managed subnet (defaults to `192.168.127.1`).
   - `OVERHYPED_RUNTIME_DIR` / `OVERHYPED_LOG_DIR` – directories for Cloud Hypervisor sockets & logs (default `~/.overhyped/run`, `~/.overhyped/logs`).
   - `overhyped_API_BASE` – CLI base URL for the control plane (default `http://127.0.0.1:7777`).
3. Build binaries via `make build-server`, `make build-agent`, or `make build-cli`.
4. Run `hype setup --dry-run` to preview host configuration, then `sudo hype setup` to create the bridge/NAT rules and systemd unit.
5. Build initramfs + kernel with `./build/images/build-initramfs.sh` (see `docs/image-pipeline.md`).

> **CGO:** The SQLite driver (`github.com/mattn/go-sqlite3`) requires CGO. Ensure Xcode command-line tools or an equivalent GCC toolchain is installed before building.

> **Note:** Module dependencies (`cobra`, `bubbletea`) require network access for `go mod tidy`. Run the command once network restrictions are lifted to populate `go.sum` and vendor caches.

## Next Implementation Targets
- Expose `/api/v1` lifecycle endpoints via authenticated gateways (middleware, auth, rate limiting).
- Harden tap/bridge management with diagnostics and host preflight checks.
- Deliver the Docker → initramfs pipeline with `overhyped-init` and agent integration.
- Wire the REST API into the CLI/TUI clients and begin MCP/AG-UI protocol adapters.

## Contributing
Project OVERHYPED is developed with a production-first mindset. Contributions should include:
- Comprehensive tests for new behavior.
- Documentation updates for user-facing or architectural changes.
- Operational considerations (observability, failure handling, resiliency).

Please consult the roadmap and development tracker before embarking on significant workstreams.
## HTTP API (Preview)
- `GET /api/v1/vms` – list managed microVMs.
- `POST /api/v1/vms` – create a microVM (`{ "name", "cpu_cores", "memory_mb", "kernel_cmdline" }`).
- `GET /api/v1/vms/:name` – fetch microVM details.
- `DELETE /api/v1/vms/:name` – destroy a microVM.
- `GET /api/v1/events/vms` – Server-Sent Events stream of lifecycle updates (`VM_CREATED`, `VM_RUNNING`, `VM_STOPPED`, `VM_CRASHED`, `VM_DELETED`).

Authentication & authorization are not yet implemented; endpoints are for local development only. `OVERHYPED_API_ALLOW_CIDR` and `OVERHYPED_API_KEY` enable lightweight network/key filtering (see `docs/auth.md`).

## CLI (Preview)
- `hype vms list` – list known microVMs (uses `OVERHYPED_API_BASE`).
- `hype vms create my-vm --cpu 2 --memory 2048` – create a new microVM.
- `hype vms get my-vm` – inspect VM status.
- `hype vms delete my-vm` – destroy a VM.
- `hype vms watch` – stream lifecycle events in real-time.
