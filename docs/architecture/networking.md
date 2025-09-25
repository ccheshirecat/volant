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

Volant **does not use DHCP.** A DHCP server would add unnecessary complexity and a potential point of failure.

Instead, the `volantd` acts as a **deterministic IP Address Management (IPAM)** authority.

1.  **The IP Pool:** The server manages the entire `192.168.127.0/24` subnet.
2.  **Transactional Leases:** When a new VM is created, the server atomically leases the next available IP address from the pool and records the lease in its SQLite database.
3.  **Kernel Command Line Injection:** This leased static IP is passed directly to the microVM via the kernel command line (e.g., `ip=192.168.127.5::192.168.127.1:24...`).
4.  **In-VM Configuration:** The `/init` script inside the VM parses this command line parameter and uses it to statically configure its `eth0` network interface.
5.  **Atomic Release:** When the VM is destroyed, the IP lease is atomically released back into the pool in the same database transaction.

This tightly coupled system is simpler, faster, and more reliable than any DHCP-based approach.

## The Agent Proxy

Clients (like the CLI/TUI) **never** talk to the microVMs directly, though you can if you need to. All communication is proxied through the `volantd` server.

```
[CLI] -> [volantd API] -> [Agent in microVM on vbr0]
```

This provides a single, secure, and authenticated entry point for the entire platform and abstracts away the entire network topology from the end-user.