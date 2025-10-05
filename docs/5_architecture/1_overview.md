# Architecture Overview

This overview is based on the actual Volant source code. It explains how the control plane, orchestrator, networking, runtime launcher, and agent cooperate to turn a plugin manifest into a running microVM.

References to source files are included so you can cross‑check details.

## High‑Level Diagram

Control Plane (volantd)
- HTTP API server: internal/server/httpapi/httpapi.go
- OpenAPI: internal/server/httpapi/openapi.go
- Orchestrator: internal/server/orchestrator/orchestrator.go
- Data store: internal/server/db
- Event bus (VM events): internal/server/eventbus

Guest (microVM)
- Agent (kestrel): cmd/kestrel, internal/agent
- Workload proxy (HTTP) and actions surface via agent

Builder (external)
- Fledge converts OCI images or scratch roots into boot media and emits a plugin manifest consumed by Volant.

## Core Concepts

- Plugin Manifest (internal/pluginspec/spec.go)
  - Declares workload, resources, networking defaults, cloud‑init, actions, and boot media (exactly one of initramfs or rootfs).
  - Serialized into kernel cmdline (volant.manifest) via base64+gzip (Encode/Decode).

- VM Config (internal/server/orchestrator/vmconfig)
  - Per‑VM/deployment overrides and recorded history. The orchestrator merges manifest defaults with config overrides at runtime.

- Runtime Launcher (internal/server/orchestrator/runtime)
  - Abstracts the hypervisor process. The orchestrator builds a LaunchSpec and calls Launcher.Launch to start a VM.

- Networking
  - Bridge manager and tap devices: internal/server/orchestrator/network
  - Modes: vsock, bridged, dhcp (see pluginspec.NetworkMode). The orchestrator only allocates IP and prepares a tap when needed.

- Cloud‑init (internal/server/orchestrator/cloudinit)
  - If configured, a NoCloud seed image is constructed and attached as a read‑only disk.

- VFIO Passthrough (internal/server/devicemanager)
  - Validates/binds PCI devices, surfaces /dev/vfio/ group device paths to the hypervisor.

## Control Plane Flow

1) Client installs a plugin manifest via API or CLI
   - Stored in DB (internal/server/db) and registered in an in‑memory registry.

2) Create VM
   - API: POST /api/v1/vms or deployments
   - httpapi parses request, resolves manifest, and calls orchestrator.CreateVM.

3) Orchestrator.CreateVM (internal/server/orchestrator/orchestrator.go)
   - Validates inputs and runtime identity
   - Allocates IP (if bridged/DHCP) and vsock CID
   - Derives MAC, kernel cmdline, and normalizes config
   - Prepares cloud‑init seed (optional)
   - Prepares tap (if required by network mode)
   - Builds runtime.LaunchSpec: CPU/mem, boot media, disks, vfio devices, networking, serial socket
   - Encodes manifest and passes kernel args (runtime, api_host, api_port, etc.)
   - Launches VM via Launcher and persists runtime state (running + PID)
   - Emits VM events on the bus

4) Agent (kestrel) boots inside guest
   - Reads kernel args, decodes manifest, sets up workload environment
   - Exposes an HTTP API for actions, logs, and OpenAPI
   - httpapi proxies selected requests to the agent (e.g., /vms/:name/agent/*)

5) Lifecycle
   - Stop/Restart: orchestrator tears down/launches processes, cleans taps, maintains state
   - Destroy: releases IP, deletes records, removes cloud‑init seed, unbinds VFIO devices

## Deployments

- Desired/ready replicas managed by orchestrator reconcile loop
- Naming: <deployment>-<n> (replicaName/parseReplicaIndex)
- Scaling adjusts DB records and VM instances accordingly

## Networking Details

- Setup utility (internal/setup/setup.go) creates bridge vbr0, configures NAT, enables ip_forward, and writes a systemd unit for volantd.
- Orchestrator only prepares a tap when the network mode requires it (needsTapDevice), and cleans it on stop/exit.

## Boot Media and Kernels

- Initramfs strategy
  - Launcher prefers a pristine vmlinux when Initramfs is present unless KernelOverride is set.
  - Manifest.Initramfs.url is required; checksum optional.

- RootFS strategy
  - Launcher uses bzImage and attaches a root disk image.
  - Manifest.RootFS.url is required; checksum optional; default device/fstype = vda/ext4 if not provided.

## Security and Isolation

- MicroVM isolation via Cloud Hypervisor
- API hardening via VOLANT_API_KEY and VOLANT_API_ALLOW_CIDR (httpapi middleware)
- VFIO passthrough uses explicit allowlist and binding steps; unbound on destroy

## Events and Observability

- Server‑Sent Events stream at /api/v1/events/vms publishes lifecycle and log events
- Agent logs can be proxied via websocket (vmLogsWebSocket)

## Data Model (high level)

- VirtualMachines: runtime state, sockets, IP, CPU/mem, kernel cmdline
- VMConfigs: versioned config snapshots
- VMGroups: deployments (desired replicas + base config)
- Plugins: installed manifests and enabled flag
- IPAllocations: simple IPAM for managed subnet

## Where to Look in Code

- HTTP API: internal/server/httpapi/httpapi.go
- OpenAPI generation: internal/server/httpapi/openapi.go
- Orchestrator engine: internal/server/orchestrator/orchestrator.go
- Runtime interface: internal/server/orchestrator/runtime/runtime.go
- Network bridge/tap: internal/server/orchestrator/network
- Cloud‑init builder: internal/server/orchestrator/cloudinit
- Plugin spec: internal/pluginspec/spec.go
- Setup utility: internal/setup/setup.go
