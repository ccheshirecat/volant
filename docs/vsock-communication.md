# Vsock Communication for Volant

## Overview

For VMs configured with vsock-only networking (no IP networking), Volant provides a vsock client library to communicate directly with the volant agent running inside the VM.

## Implementation

### Vsock Client Library

**Location**: `internal/vsock/client.go`

The vsock client provides HTTP-over-vsock communication:

```go
vsockClient := vsock.NewClient(cid, port)
err := vsockClient.DoJSON(ctx, "POST", "/v1/action", requestBody, &responseBody)
```

### CLI Client Integration

**Location**: `internal/cli/client/client.go`

The CLI client exposes two methods for vsock communication:

1. **AgentRequestVsock** - Full control over CID and port:
```go
func (c *Client) AgentRequestVsock(ctx context.Context, cid, port uint32, method, path string, body any, out any) error
```

2. **AgentRequestVsockDefault** - Uses default CID (3) and port (8080):
```go
func (c *Client) AgentRequestVsockDefault(ctx context.Context, method, path string, body any, out any) error
```

## Usage Example

```go
// Create CLI client
client, _ := client.New("http://localhost:7777")

// Invoke action over vsock (CID 3, port 8080)
var result map[string]interface{}
err := client.AgentRequestVsockDefault(
    context.Background(),
    "POST",
    "/v1/plugin/my-plugin/actions/process",
    map[string]string{"input": "data"},
    &result,
)
```

## Default Configuration

- **CID**: 3 (configured in Cloud Hypervisor launcher)
- **Port**: 8080 (volant agent default vsock listen port)

## Architecture

```
CLI/API Client
    ↓
vsock.Client (HTTP-over-vsock)
    ↓
Host vsock device (/dev/vsock)
    ↓
VM vsock device (CID 3)
    ↓
Volant Agent (listening on vsock:8080)
    ↓
Plugin Actions
```

## Future Enhancements

- [ ] Automatic CID discovery from VM metadata
- [ ] Support for multiple vsock ports per VM
- [ ] Vsock device management and CID allocation
- [ ] CLI commands that auto-detect network mode and use vsock when appropriate
- [ ] Health checks over vsock

## See Also

- [Network Configuration](./network-configuration.md) - Network modes and configuration
- Cloud Hypervisor vsock documentation
