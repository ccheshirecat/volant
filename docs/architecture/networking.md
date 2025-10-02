---
title: "Networking Model"
description: "Volant's opinionated, secure, and zero-configuration network architecture."
---

# Networking Model

Volant's networking is built on a simple, powerful, and opinionated philosophy: **it should just work, and it should be secure by default.** We reject the complexity of traditional virtualization networking and provide a managed, isolated environment out of the box.

## The `vbr0` Bridge

The heart of Volant's networking is the `vbr0` Linux bridge, which is automatically created and configured by the `volar setup` command.

- **Private Subnet:** The bridge is assigned a static IP address on a dedicated private subnet, by default `192.168.127.1/24`. All microVMs will live on this network.
- **NAT for Internet Access:** `volar setup` automatically configures `iptables` `MASQUERADE` rules. This allows all microVMs to access the outbound internet for tasks like browsing websites, but prevents any unsolicited inbound traffic from reaching the VMs.
- **Host Access:** The host machine can directly communicate with any VM on the `vbr0` bridge.

## Managed Static IP Allocation

By default, Volant uses deterministic, server-managed static IPs. The `volantd` acts as a **deterministic IP Address Management (IPAM)** authority. DHCP is supported when explicitly requested by a plugin/VM network config, and a vsock-only mode is available for air‑gapped workloads.

1.  **The IP Pool:** The server manages the entire `192.168.127.0/24` subnet.
2.  **Transactional Leases:** When a new VM is created, the server atomically leases the next available IP address from the pool and records the lease in its SQLite database.
3.  **Kernel Command Line Injection:** This leased static IP is passed directly to the microVM via the kernel command line (e.g., `ip=192.168.127.5::192.168.127.1:24...`).
4.  **In-VM Configuration:** The `/init` script inside the VM parses this command line parameter and uses it to statically configure its `eth0` network interface.
5.  **Atomic Release:** When the VM is destroyed, the IP lease is atomically released back into the pool in the same database transaction.

This tightly coupled system is simple, fast, and reliable. When DHCP mode is selected, Volant attaches the VM to the bridge but does not allocate or manage IPs; the guest obtains an address from your DHCP server. In vsock‑only mode, no tap device is created and no IP is assigned.

## The Agent Proxy

Clients (like the CLI/TUI) **never** talk to the microVMs directly, though you can if you need to. All communication is proxied through the `volantd` server.

```
[CLI] -> [volantd API] -> [Agent in microVM on vbr0]
```

This provides a single, secure, and authenticated entry point for the entire platform and abstracts away the entire network topology from the end‑user.

## Network Modes

Volant supports three modes via plugin or per‑VM configuration:

- vsock: Isolated vsock‑only communication (no IP). The hypervisor configures a vsock device and Volant assigns a unique CID to each VM.
- bridged: VM is attached to the `vbr0` bridge with host‑managed static IPs (default).
- dhcp: VM is attached to the `vbr0` bridge and acquires IP via your DHCP server; Volant does not manage addresses in this mode.

See also: Network configuration examples below. Content from the previous standalone docs/network-configuration.md and docs/vsock-communication.md has been consolidated here.

### Network Configuration Examples

Plugin manifest with vsock-only networking:

```
{
  "name": "secure-worker",
  "version": "1.0.0",
  "network": {
    "mode": "vsock"
  }
}
```

Bridged networking with auto-assign:

```
{
  "name": "web-service",
  "version": "1.0.0",
  "network": {
    "mode": "bridged",
    "subnet": "192.168.127.0/24",
    "gateway": "192.168.127.1",
    "auto_assign": true
  }
}
```

Per-VM override to DHCP:

```
{
  "plugin": "web-service",
  "network": {
    "mode": "dhcp"
  }
}
```