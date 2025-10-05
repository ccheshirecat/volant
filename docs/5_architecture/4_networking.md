# Networking Model

Volant supports three network modes declared in the plugin manifest (internal/pluginspec/spec.go) and overridable per-VM via vmconfig.Config.

## Modes

- vsock
  - No IP networking; host↔guest comms via virtio-vsock only
  - Orchestrator: no tap created; no IP allocated
  - Launcher: configures --vsock cid=<cid>

- bridged
  - Host-managed Linux bridge (vbr0) with a per-VM tap device
  - Orchestrator: allocates IP and prepares tap; attaches tap to bridge
  - Launcher: configures --net tap=<tap>,mac=<mac>,ip=<ip>,mask=<netmask>

- dhcp
  - Similar to bridged, but guest obtains IP via DHCP
  - Orchestrator: prepares tap; no host IP allocation
  - Launcher: configures --net tap=<tap>,mac=<mac> (no ip/mask)

## Bridge/Tap Provisioning

- Code: internal/server/orchestrator/network/bridge.go (linux)
  - Creates tuntap device with TUNTAP_VNET_HDR and attaches it to the bridge
  - Naming: vttap-<sanitized-name-or-hash>, constrained to IFNAMSIZ 15 chars
- Non-Linux builds use a noop manager (bridge_stub.go → NewNoop())

## Setup Script

- Code: internal/setup/setup.go
  - Creates bridge vbr0, assigns host CIDR, brings it up
  - Enables IP forwarding and sets NAT masquerade for the subnet

## Decision Logic (when to create taps, allocate IPs)

- Tested in internal/server/orchestrator/network_test.go
- needsTapDevice(cfg): true for bridged/dhcp; false for vsock; default true if cfg nil/empty
- needsIPAllocation(cfg): true for bridged; false for dhcp/vsock; default true if cfg nil/empty
