# volantd (server) flags

Source: cmd/volantd/main.go, internal/server/config.

volantd reads most settings from environment variables via internal/server/config.FromEnv(). Key flags/vars:

- VOLANT_API_LISTEN_ADDR: host:port to bind (default 127.0.0.1:7777)
- VOLANT_API_ADVERTISE_ADDR: advertised host:port for clients (defaults to listen addr)
- VOLANT_SUBNET: managed subnet CIDR (default 192.168.127.0/24)
- VOLANT_HOST_IP: host IP inside the bridge (default 192.168.127.1)
- VOLANT_RUNTIME_DIR: runtime directory (~/.volant/run by default)
- VOLANT_LOG_DIR: logs directory (~/.volant/logs by default)
- VOLANT_BRIDGE: Linux bridge name (default vbr0)
- VOLANT_KERNEL_BZIMAGE: bzImage path for rootfs strategy
- VOLANT_KERNEL_VMLINUX: vmlinux path for initramfs strategy
- VOLANT_DB_PATH: sqlite database path
- VOLANT_HYPERVISOR: cloud-hypervisor binary path (default: cloud-hypervisor)

On Linux, the server selects the bridge-backed network manager. On non-Linux, it warns and falls back to a no-op network manager.
