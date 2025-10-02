# Architecture Overview

## System Components
- **volantd**: Native orchestrator responsible for lifecycle management of Cloud Hypervisor microVMs, static IP allocation, plugin manifest registry, and API exposure (REST, MCP).
- **volary**: In-VM Go agent that hydrates the runtime declared by the manifest (no longer hard-wired to Chrome). It mounts plugin-defined HTTP/WebSocket routes and manages optional DevTools/log streaming based on the workload contract.
- **Plugin Runtime Artifacts**: Signed manifests, rootfs bundles, and optional OCI images distributed per plugin. The orchestrator injects manifest payloads into the VM kernel cmdline and mounts artifacts from the runtime directory.
- **Client Tooling**: `volar` CLI providing operator and automation entry points.

## Control Flow Summary
1. **VM Creation**
   - REST/MCP request hits `volantd`.
   - Server begins SQLite transaction, leases deterministic IP, and persists VM metadata.
   - Engine loads the plugin manifest, encodes it into kernel cmdline parameters (`volant.manifest`, `volant.runtime`, `volant.plugin`), and launches Cloud Hypervisor with the declared rootfs/kernel artifacts.
- Event bus broadcasts VM lifecycle event for subscribers.

2. **In-VM Boot**
   - Kernel executes `/bin/volant-init` which mounts virtual filesystems, parses `ip=` from `/proc/cmdline`, configures `eth0`, and `exec`s `volary`.
   - Agent decodes the manifest, hydrates the declared workload (entrypoint, environment, base URL), launches the process, and exposes whatever interfaces the manifest describes (REST, WebSocket, DevTools, etc.). No legacy browser assumptions remain.

3. **Task Execution**
   - CLI/MCP clients call workload endpoints directly based on manifest metadata. Legacy action proxies (`/api/v1/plugins/{plugin}/actions/{action}`) remain for older manifests but are no longer required.
   - Results, logs, and artifacts stream back through the proxy and event bus.

4. **Teardown**
   - Destroy request terminates Cloud Hypervisor process, updates state, and releases IP within the same SQLite transaction.

## Persistence
- Embedded SQLite at `$HOME/.volant/state.db` managed via the in-process `sqlite` store (`internal/server/db/sqlite`) using the CGO-backed `github.com/mattn/go-sqlite3` driver.
- Go-embedded migrations execute sequentially at boot, recording state in `schema_migrations` for deterministic upgrades.
- Typed repositories provide CRUD and lifecycle helpers:
  - `VirtualMachines()` handles creation, runtime status updates (PID/status), kernel cmdline updates, and deletion.
  - `IPAllocations()` manages pool seeding (`EnsurePool`), deterministic leasing, VM assignment, and release semantics.
- `Plugins()` persists manifests, tracks enablement state, and stores the encoded manifest payload for injection.
- Core tables:
  - `vms(id, name, status, pid, ip_address, cpu_cores, memory_mb, created_at, updated_at)`
  - `ip_allocations(ip_address PRIMARY KEY, vm_id NULLABLE, status, leased_at)`
- `plugins(id, name, version, runtime, manifest_json, enabled, created_at, updated_at)`
- `Store.WithTx` coordinates transactional workflows so IP leases and VM lifecycle mutations commit atomically.

## Orchestrator Engine (Current State)
- `internal/server/orchestrator` exposes a production engine constructor with dependency injection for `db.Store`, logging, and subnet metadata.
- `Engine.Start` seeds the IP pool via `EnsurePool`, guaranteeing availability before servicing requests.
- `CreateVM` performs validation, resolves the plugin manifest, leases the next available static IP, assigns a deterministic MAC (stable hash of name+IP), persists metadata (including runtime and manifest digest), and delegates to the runtime launcher to boot the VM. A background monitor watches the hypervisor process and marks the VM `stopped`/`crashed` while cleaning up taps and sockets.
- `DestroyVM` tears down VM metadata and releases the associated IP in the same transaction.
- Public queries (`ListVMs`, `GetVM`) surface persisted state ahead of REST exposure.
- A pluggable runtime layer (`internal/server/orchestrator/runtime`) defines the launch contract; the `cloudhypervisor.Launcher` assembles the `cloud-hypervisor` command, manages API sockets/logs, injects manifest kernel parameters, and exposes graceful shutdown hooks.
- Lifecycle changes emit structured events on `eventbus.Bus` (`internal/server/orchestrator/events`), enabling REST/MCP layers to stream `VM_CREATED`, `VM_RUNNING`, `VM_STOPPED`, and `VM_CRASHED` notifications.

## Networking Model
- Host bridge `vbr0` at `192.168.127.1/24` created by `volar setup`.
- NAT enabled via `iptables` MASQUERADE to allow outbound access.
- Default: server-managed static IPs injected via kernel cmdline; DHCP is supported when requested by a plugin/VM config, and a vsock-only mode skips IP networking entirely.
- MAC addresses generated per VM with deterministic prefix to simplify filtering.
- `network.BridgeManager` provisions tap interfaces via `ip tuntap`/`ip link`, attaches them to the bridge, and tears them down during VM destruction. A `NoopManager` remains available for non-Linux development hosts.

## API Layer
- `internal/server/httpapi` uses Gin to expose REST endpoints at `/api/v1`:
  - `GET /api/v1/vms` lists orchestrated microVMs.
  - `POST /api/v1/vms` creates a VM (CPU/memory/cmdline payload).
  - `GET /api/v1/vms/:name` retrieves a single VM.
  - `DELETE /api/v1/vms/:name` destroys a VM and releases its resources.
  - `GET /api/v1/system/status` returns system metrics (VM count, CPU/MEM placeholders).
- `GET /api/v1/events/vms` streams lifecycle events over Server-Sent Events (SSE) sourced from the internal event bus.
- Plugin-aware endpoints extend the surface: `/api/v1/plugins` manages manifest lifecycle. Historical action proxy endpoints remain for backwards compatibility, but new clients should consume the manifest's published OpenAPI metadata and call workloads directly.
- Request/response payloads are JSON; future work will introduce authn/z, pagination, and richer error semantics.

## Eventing & Protocols
- Internal event bus fan-outs lifecycle events to REST SSE subscribers and VM WebSocket endpoints (e.g., logs, DevTools).
- MCP handler for AI orchestration. Plugin manifests are exposed via the protocol for discovery so agents can determine required runtimes before execution.
- Future work: durable event log for replay/audit.

## Security & Observability
- API access control: optional API token (`VOLANT_API_KEY`) validated via `X-Volant-API-Key` header or `api_key` query parameter; optional CIDR allow-list via `VOLANT_API_ALLOW_CIDR`.
- For production deployments, run behind TLS-terminating proxy; future enhancements may include mTLS/OIDC.
- Structured logging via `log/slog`; metrics/exporters to be added (Prometheus/OpenTelemetry).
- Agent-server communication constrained to private subnet; host firewall rules enforced by installer.

## Open Questions
- Migration tooling selection (golang-migrate vs. sqlc w/ migrations?).
- Artifact distribution strategy for kernel/initramfs (local cache vs. remote registry).
- Secret management for profile injection (encryption at rest, retrieval flows).

This document evolves alongside implementation milestones; keep it updated as contracts solidify.
