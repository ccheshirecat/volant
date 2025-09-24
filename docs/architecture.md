---
title: Architecture
description: System architecture and component overview
---

# Architecture Overview

## Core Systems

### viper-server
- Embedded orchestrator written in Go.
- Owns lifecycle of Cloud Hypervisor microVMs (create, monitor, destroy).
- Maintains authoritative state in SQLite (`~/.viper/state.db`).
- Exposes REST, MCP (`POST /api/mcp`), and AG-UI (`GET /ws/v1/agui`) surfaces.
- Houses the DevTools proxy (`/api/v1/vms/{name}/agent`) so clients never hit guest IPs directly.

### viper-agent
- Runs inside each microVM as PID 1 after `/bin/viper-init` hands off.
- Launches headless Chrome with a per-instance user-data dir and devtools port.
- Presents REST endpoints that mirror browser automation primitives.
- Streams logs/artifacts back via the control plane proxy.
- Observability: structured logs, exit watchdog, metrics hooks (roadmap).

### Image Pipeline
- `build/images/build-initramfs.sh` builds a Docker image with chrome + agent + `viper-init`.
- Exports `vmlinux` and `viper-initramfs.cpio.gz` for the orchestrator.
- Make target `build-images` fixes checksums and ensures artifacts exist.

### Client Surfaces
- CLI (`cmd/viper`): Cobra commands for automation and `cobra` friendly UX.
- TUI: Bubble Tea app connecting through REST/SSE for real-time status.
- Programmatic: REST, MCP, AG-UI, and DevTools proxy over HTTP/WebSockets.

## Control Plane Lifecycle

### 1. VM Creation
1. Client issues REST/MCP call.
2. Server begins SQLite transaction, leases next static IP, and persists VM metadata.
3. `cloudhypervisor.Launcher` stages dedicated kernel/initramfs copies and starts `cloud-hypervisor` with static `ip=` cmdline and tap device.
4. Event bus emits `VM_CREATED` → SSE/TUI/AG-UI listeners update immediately.

### 2. Boot & Agent Initialization
1. Kernel boots with `init=/init` (`viper-init`).
2. `viper-init` mounts pseudo filesystems, configures networking, and supervises `viper-agent`.
3. Agent spawns Chrome (per-instance port/profile) and announces readiness via the control plane.

### 3. Task Execution
1. All automation endpoints (`/api/v1/vms/{name}/agent/...`) tunnel through the server.
2. Outgoing requests are signed/authorized (roadmap) and forwarded into the VM.
3. Responses/logs are streamed back to the caller, while the event bus updates the TUI/AG-UI feed.

### 4. Teardown
1. Destroy request terminates hypervisor process.
2. Transaction releases the IP lease and deletes VM record.
3. Tap device and per-instance artifacts (kernel/initramfs copies, user-data dir) are removed.
4. Event bus emits `VM_STOPPED` or `VM_CRASHED` depending on exit status.

## Persistence & Data Model
- **Database:** SQLite via `mattn/go-sqlite3`; migrations embedded and applied on boot.
- **Tables:**
  - `vms`: ID, name, status, PID, CPU/memory, IP, timestamps.
  - `ip_allocations`: IP, VM association, lease metadata.
  - `plugins`, `workloads`: reserved for extension modules.
- **Transactions:** `Store.WithTx` ensures IP leases and VM state mutate atomically.
- **Event Log:** In-memory publish/subscribe now; durable storage on the roadmap.

## Networking Model
- `viper setup` creates bridge `viperbr0` (`192.168.127.1/24`) and configures NAT (iptables `MASQUERADE`).
- MicroVMs receive static IP via kernel cmdline (`ip=a.b.c.d::gateway:netmask:hostname:eth0:off`).
- MAC addresses derived from SHA-1 of `name|ip` to keep them deterministic.
- `BridgeManager` provisions tap interfaces, attaches them to bridge, cleans them up on teardown.
- DevTools ports are bound to `127.0.0.1` and proxied by the server, never exposed on the guest network.

## API Surfaces (High-Level)
- **REST:** `/api/v1/...` for VM lifecycle, status, logs, and agent proxy.
- **Agent REST (proxied):** Browser navigation, DOM operations, profiles, screenshots.
- **Events:** SSE (`/api/v1/events/vms`) + WebSocket for AG-UI.
- **MCP:** `POST /api/mcp` maps commands (e.g., `viper.vms.create`) to orchestrator actions.
- **AG-UI:** `GET /ws/v1/agui` streams run events for AI/UI clients.

See the dedicated API reference section for the full OpenAPI spec and protocol schemas.

## Observability
- Structured logging via `log/slog` on the server, channeled into SSE and AG-UI.
- Agent anonymized logs forwarded through the server (captured from Chrome stdout/stderr).
- Event bus instrumentation on the roadmap (Prometheus/OpenTelemetry exporters).
- TUI surfaces live VM/log streams; API exposes `/api/v1/system/status` for health.

## Extensibility
- Pluggable runtime interface allows future hypervisors or container back-ends.
- MCP/AG-UI command adapters open protocol integration with AI systems.
- Planned plugin system (stored in `plugins` table) for workload automation modules.

Keep this page current as implementation evolves; treat it as the canonical blueprint for Viper operators and contributors.