# Troubleshooting

## Build on macOS fails with netlink TUNTAP constants

Issue: undefined: netlink.TUNTAP_MODE_TAP (and related) when building tools like openapi-export on macOS.

Cause: internal/server/orchestrator/network/bridge.go uses Linux-only netlink constants. The file is build-tagged for linux, so ensure your local copy has:

```go
//go:build linux
// +build linux
```

at the top of bridge.go. On non-Linux, volantd uses the Noop network manager.

Workaround for building Linux-only binaries on macOS:

```bash
GOOS=linux GOARCH=amd64 make build
```

## OpenAPI export path

Use:

```bash
make openapi-export
```

It builds bin/openapi-export and writes docs/api-reference/openapi.json with the server URL set to https://docs.volantvm.com.

## Networking not working on Linux

Run `volar setup` as root to create vbr0, assign 192.168.127.1/24, enable IP forwarding, and add iptables rules. See internal/setup/setup.go for exact commands (use --dry-run first).

## No IP address on VM

- If using vsock mode: expected (no Ethernet). Interact via agent proxy.
- If using dhcp mode: the VM must run DHCP client; host won’t assign IP.
- If bridged: ensure IP pool is available and guest kernel cmdline is applied.
