# Network Configuration with Multiple Modes

**Status**: Schema implementation complete (Task #7)  
**Author**: Droid  
**Date**: 2025-01-06

## Overview

This document describes the network configuration feature that enables plugins and VMs to specify their networking requirements with support for multiple network modes: vsock-only (isolated), bridged (manual or auto IP assignment), and DHCP-based networking.

## Architecture

### Network Modes

1. **vsock** - Isolated communication via vsock socket (no IP networking)
   - VM communicates with host only via vsock
   - No tap device, no IP address allocation
   - Suitable for secure, isolated workloads

2. **bridged** - VM attached to Linux bridge with IP networking
   - Requires subnet and gateway configuration
   - Supports manual IP assignment or auto-allocation from subnet
   - Full IP connectivity within the subnet

3. **dhcp** - VM attached to bridge with DHCP-based IP assignment
   - VM obtains IP from external DHCP server
   - Bridge must be connected to network with DHCP service
   - No host-side IP management required

### Configuration Hierarchy

Network configuration follows a two-level hierarchy:

1. **Plugin-level defaults** (in plugin manifest)
   - Defined by plugin author
   - Specifies the default network mode for all VMs of this plugin
   - Can include subnet, gateway, and auto-assign settings

2. **Per-VM overrides** (in VM config)
   - Defined by VM operator
   - Overrides plugin-level defaults for specific VM instance
   - Allows customization without modifying plugin manifest

## Schema

### Plugin Manifest (`pluginspec.Manifest`)

```go
type Manifest struct {
    // ... existing fields ...
    Network *NetworkConfig `json:"network,omitempty"`
    // ... remaining fields ...
}
```

### VM Config (`vmconfig.Config`)

```go
type Config struct {
    // ... existing fields ...
    Network *pluginspec.NetworkConfig `json:"network,omitempty"`
    // ... remaining fields ...
}
```

### Network Configuration Type

```go
// NetworkMode defines how the VM connects to the network.
type NetworkMode string

const (
    NetworkModeVsock   NetworkMode = "vsock"   // Isolated vsock-only
    NetworkModeBridged NetworkMode = "bridged" // Bridge with IP networking
    NetworkModeDHCP    NetworkMode = "dhcp"    // Bridge with DHCP
)

// NetworkConfig defines network configuration.
type NetworkConfig struct {
    Mode       NetworkMode `json:"mode"`
    Subnet     string      `json:"subnet,omitempty"`      // For bridged: CIDR (e.g., "10.1.0.0/24")
    Gateway    string      `json:"gateway,omitempty"`     // For bridged: gateway IP
    AutoAssign bool        `json:"auto_assign,omitempty"` // For bridged: auto-allocate IPs
}
```

## Implementation Status

### ✅ Completed

1. **Schema Definition** (`internal/pluginspec/spec.go`)
   - Added `NetworkMode` type and constants
   - Added `NetworkConfig` struct with validation
   - Added `Network` field to `Manifest`
   - Implemented `Normalize()` and `Validate()` methods

2. **VM Config Integration** (`internal/server/orchestrator/vmconfig/config.go`)
   - Added `Network` field to `Config` struct
   - Added `Network` field to `Patch` struct
   - Updated `Clone()`, `Normalize()`, `Validate()`, and `Apply()` methods

3. **Validation Logic**
   - Mode validation (must be vsock, bridged, or dhcp)
   - Bridged mode validation (subnet + gateway or auto_assign required)
   - DHCP and vsock modes require no additional config

### 🔄 In Progress / Pending

4. **Orchestrator Engine Integration**
   - [ ] Update `network.Manager` interface to support vsock-only mode
   - [ ] Modify VM start logic to conditionally prepare tap based on network mode
   - [ ] Skip IP allocation for vsock mode
   - [ ] Add vsock CID management for vsock-only VMs
   - [ ] Implement subnet/IP allocation logic for bridged auto-assign mode
   - [ ] Add DHCP mode handling (no host-side IP management)

5. **Runtime/Launcher Updates**
   - [ ] Update firecracker launcher to configure vsock when in vsock mode
   - [ ] Pass network mode to VM launch configuration
   - [ ] Configure guest network interfaces based on mode

6. **CLI Support**
   - [ ] Add `--network-mode` flag to VM creation commands
   - [ ] Add `--network-subnet` and `--network-gateway` flags for bridged mode
   - [ ] Add `--network-auto-assign` flag for bridged auto-allocation
   - [ ] Update VM config patch commands to support network changes

7. **API Updates**
   - [ ] Expose network configuration in VM and plugin API responses
   - [ ] Add validation for network config in API handlers

8. **Testing**
   - [ ] Unit tests for NetworkConfig validation
   - [ ] Integration tests for each network mode
   - [ ] Test plugin-level defaults vs VM overrides
   - [ ] Test network mode transitions (e.g., bridged -> vsock)

## Usage Examples

### Plugin Manifest with vsock-only Networking

```json
{
  "name": "secure-worker",
  "version": "1.0.0",
  "network": {
    "mode": "vsock"
  },
  ...
}
```

### Plugin Manifest with Bridged Networking

```json
{
  "name": "web-service",
  "version": "1.0.0",
  "network": {
    "mode": "bridged",
    "subnet": "10.100.0.0/24",
    "gateway": "10.100.0.1",
    "auto_assign": true
  },
  ...
}
```

### VM Config Override to DHCP

```json
{
  "plugin": "web-service",
  "network": {
    "mode": "dhcp"
  },
  ...
}
```

## Migration Strategy

### Backward Compatibility

- If `Network` is `nil` or not specified, default to current behavior (bridged with host-managed IPs)
- Existing VMs continue to work without changes
- Plugin authors can opt-in to network configuration

### Recommended Migration Path

1. Start with schema-only changes (current PR)
2. Add orchestrator support for vsock mode
3. Add bridged auto-assign support
4. Add DHCP mode support
5. Update CLI and API
6. Add comprehensive tests

## Security Considerations

- **vsock mode** provides strongest isolation (no network stack in VM)
- **bridged mode** requires careful subnet planning to avoid IP conflicts
- **dhcp mode** trusts external DHCP server for IP assignment
- Validate all network configurations before VM creation
- Enforce network mode restrictions per plugin requirements

## Future Enhancements

- Multiple network interfaces per VM
- Custom bridge selection per VM
- Network policies and firewall rules
- IPv6 support
- Container networking integration (CNI plugins)

## References

- Task #7: Network Configuration with Multiple Modes
- Firecracker networking documentation
- Linux bridge and vsock documentation
