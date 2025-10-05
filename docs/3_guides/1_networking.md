# Networking guide

Ground truth: internal/server/orchestrator/orchestrator.go (resolveNetworkConfig, needsIPAllocation, needsTapDevice), internal/server/orchestrator/network/{bridge.go,noop.go}, internal/setup/setup.go.

Volant supports three network modes via the plugin manifest (or per‑VM config override):
- bridged (default)
- vsock
- dhcp

The effective mode is resolved by resolveNetworkConfig(manifest, vmConfig). VM‑level config overrides the manifest.

## What each mode means

- bridged:
  - Host creates a tap device and attaches it to the configured Linux bridge (vbr0 by default).
  - Orchestrator allocates an IP from the managed subnet and programs the VM kernel cmdline with ip/gateway/netmask.
  - needsTapDevice = true; needsIPAllocation = true.

- vsock:
  - No guest Ethernet; communication via vsock only (kestrel agent proxy).
  - No tap device and no host‑managed IP.
  - needsTapDevice = false; needsIPAllocation = false.

- dhcp:
  - VM gets IP via DHCP from inside the guest; host only provides a tap/bridge.
  - needsTapDevice = true; needsIPAllocation = false.

## Host networking on Linux

- Bridge manager (internal/server/orchestrator/network/bridge.go) uses vishvananda/netlink to:
  - Ensure the bridge exists and is up
  - Create a tap interface with VNET_HDR and attach it to the bridge
  - Bring it up and hand the tap name to the runtime
- The bridge code is linux‑only (build‑tagged). On non‑Linux hosts, volantd falls back to NoopManager.

## macOS and non‑Linux hosts

- No tap devices are created automatically. volantd logs a warning and uses NoopManager which returns a deterministic tap name (no system changes).
- If you want bridged networking on macOS, create and manage the tap/bridge manually and point your runtime to it.

## Setup helper (Linux)

The CLI command `volar setup` (internal/cli/standard/setup.go) calls internal/setup/ to:
- Create bridge (default vbr0) and assign host IP (default 192.168.127.1/24)
- Enable IP forwarding and add MASQUERADE and FORWARD rules via iptables
- Optionally write a systemd unit for volantd with proper environment

You can run with --dry-run to print commands without applying them.

## Kernel cmdline and IP

For bridged mode, the orchestrator computes:
- ip=<guest_ip>::<gateway>:<netmask>:<hostname>:eth0:off
- gateway = hostIP from config
- netmask derived from subnet mask (formatNetmask)

This is passed to the runtime; the guest should configure eth0 accordingly on boot.
