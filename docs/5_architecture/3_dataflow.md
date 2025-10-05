# Data Flow Deep Dive

This page traces the request paths and artifacts through the system.

## VM Creation Path

1) API/CLI → HTTP API
   - Endpoint: POST /api/v1/vms
   - Code: internal/server/httpapi/httpapi.go:createVM
   - Resolves plugin manifest from registry; merges request + config overrides.

2) Orchestrator.CreateVM
   - Code: internal/server/orchestrator/orchestrator.go:CreateVM
   - Actions:
     - Validate request; pick runtime identity
     - Allocate IP (if bridged) and vsock CID
     - Derive MAC, kernel cmdline, serial socket
     - Prepare cloud-init seed image if configured
     - Persist VM row, config snapshot, cloud-init record (transaction)
     - Prepare tap device (if needed)
     - Build runtime.LaunchSpec and encode manifest in kernel args
     - Launch hypervisor via Launcher and update VM state to running
     - Publish events

3) Cloud Hypervisor Launch
   - Code: internal/server/orchestrator/cloudhypervisor/launcher.go:Launch
   - Stages kernel (override > vmlinux|bzImage), downloads initramfs/rootfs with sha256, assembles args and starts the process.

4) Agent Boot
   - Decodes manifest, starts workload, exposes APIs.
   - Control plane can proxy actions/logs/OpenAPI via httpapi.

## Deployment Reconciliation

- Input: VMGroups row with desired replicas and a base vmconfig.Config
- Code path:
  - internal/server/orchestrator/orchestrator.go:CreateDeployment → reconcileDeploymentByID → reconcileDeployment
  - Scales down by destroying high-index VMs first; scales up by creating missing indices (name → <group>-<n>).

## Networking Decisions

- resolveNetworkConfig(manifest, config)
  - Unit tests: internal/server/orchestrator/network_test.go
  - needsIPAllocation(cfg): bridged=true; dhcp/vsock=false; nil/empty defaults to true
  - needsTapDevice(cfg): bridged/dhcp=true; vsock=false; nil/empty defaults to true

## Cloud-Init Seed Generation

- Code: internal/server/orchestrator/cloudinit/builder.go
- Prefers cloud-localds; otherwise builds a FAT image labeled CIDATA and writes user-data/meta-data/network-config
- Seed is attached as a read-only disk in LaunchSpec

## VFIO Device Passthrough

- Code: internal/server/devicemanager/vfio_manager.go
- API: /api/v1/vfio/* endpoints in httpapi for validation/binding and group path discovery
- Orchestrator binds and unbinds devices on create/destroy; device paths added to LaunchSpec

## Persistence Model

- VM lifecycle state transitions are wrapped in Store.WithTx
- Tables and migrations: internal/server/db/sqlite/migrations/*.sql
- VM config history stored on each update; current pointer held in vm_configs
