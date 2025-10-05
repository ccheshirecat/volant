# Components and Responsibilities

This section drills into each major subsystem with references to concrete code.

## Control Plane (volantd)

- HTTP API server
  - File: internal/server/httpapi/httpapi.go
  - Framework: gin (release mode)
  - Security: optional X-Volant-API-Key and VOLANT_API_ALLOW_CIDR middleware
  - Responsibilities:
    - Validate and route requests to the orchestrator
    - Plugin registry integration (install/remove/toggle, serve manifests)
    - Reverse proxy to guest agent for actions/logs/OpenAPI
    - SSE stream for lifecycle events

- OpenAPI generation
  - File: internal/server/httpapi/openapi.go
  - Exposes /openapi with a generated spec; docs/api-reference/openapi.json is built via cmd/openapi-export

- Plugins registry
  - Files: internal/server/plugins/{registry.go, loader.go}
  - Stores runtime manifests in memory, persists to DB via db.PluginRepository
  - Action resolution and basic action dispatch path

## Orchestrator

- Engine interface and production engine
  - File: internal/server/orchestrator/orchestrator.go
  - Provides VM lifecycle (Create/Start/Stop/Restart/Destroy), deployments, config history, events
  - Merges:
    - Plugin manifest defaults (internal/pluginspec/spec.go)
    - VM config overrides (internal/server/orchestrator/vmconfig)

- Networking decision helpers
  - File: internal/server/orchestrator/orchestrator.go (resolveNetworkConfig, needsTapDevice, needsIPAllocation with tests in internal/server/orchestrator/network_test.go)
  - Modes:
    - vsock: no tap, no host-managed IP
    - bridged: tap + host-managed IP + bridge vbr0
    - dhcp: tap only; IP via guest DHCP

- Cloud-init seed
  - Files: internal/server/orchestrator/cloudinit/builder.go
  - Builds NoCloud seed via cloud-localds if available, otherwise a FAT image populated with user-data/meta-data

- VFIO device manager
  - Files: internal/server/devicemanager/vfio_manager.go
  - Validates/binds/unbinds PCI devices and resolves /dev/vfio/<group> paths

- Runtime launcher abstraction
  - Files: internal/server/orchestrator/runtime/runtime.go
  - LaunchSpec includes CPU/mem, boot media, network, cloud-init disk, args, VFIO

- Cloud Hypervisor launcher
  - Files: internal/server/orchestrator/cloudhypervisor/launcher.go
  - Selects kernel: KernelOverride > (Initramfs ? vmlinux : bzImage)
  - Streams/fetches initramfs/rootfs with checksum verification
  - Configures --net (tap, mac, ip, mask) or --vsock (cid), disks, serial socket

## Data Storage

- SQLite backend
  - Files: internal/server/db/sqlite/*.go, migrations/*
  - Tables:
    - vms: runtime state, identity, resources
    - vm_configs + vm_config_history: versioned spec snapshots
    - plugins: installed manifests (enabled flag, metadata JSON)
    - ip_allocations: simple IPAM
    - vm_groups: deployments (desired replicas, config)

- Transactions
  - File: internal/server/db/types.go (Store, Queries)
  - All orchestrator state changes are wrapped in WithTx for atomicity

## Event Bus

- Interface and in-memory impl
  - Files: internal/server/eventbus/{bus.go, memory}
  - Topics: orchestratorevents.TopicVMEvents (created, running, stopped, crashed, logs)
  - API stream: /api/v1/events/vms (SSE)

## Setup Utility

- Host bootstrap
  - File: internal/setup/setup.go
  - Creates bridge, enables ip_forward, sets NAT, writes systemd unit with kernel paths (bzImage/vmlinux)

## Command-Line Interfaces

- volantd (server)
  - File: cmd/volantd
  - Reads env for bridge/subnet/runtime/log directories and kernel paths

- volar (client)
  - Files: internal/cli/standard/*.go
  - Notable: vms create supports --kernel, --initramfs, --initramfs-checksum overrides

- openapi-export (docs)
  - File: cmd/openapi-export
  - Generates docs/api-reference/openapi.json from the live router

## Agent (Guest)

- Kestrel (lightweight agent)
  - Files: cmd/kestrel, internal/agent (selected parts)
  - Responsibilities:
    - Decode manifest from kernel cmdline (volant.manifest)
    - Start workload and expose HTTP surface for actions, logs, OpenAPI
    - Optional DevTools bridge for browser runtimes
