---
title: The Control Plane
description: Understanding the volantd daemon and the native orchestrator.
---

# The Control Plane: `volantd`

The `volantd` daemon is the central nervous system of the Volant platform. It's a single, long-running Go binary that acts as the orchestrator, API server, and state manager.

## Core Responsibilities

- **Native Orchestration:** Unlike other tools that delegate to complex, generic systems like Kubernetes or Nomad, `volantd` contains its own lightweight, special-purpose orchestrator designed exclusively for managing Cloud Hypervisor microVMs.
- **State Management:** It maintains the authoritative state of the entire cluster (VMs, IP allocations, plugins) in a local SQLite database, ensuring consistency and reliability.
- **API Server:** It exposes a unified set of interfaces (REST, MCP, AG-UI) for all clients to interact with.
- **Agent Proxy:** It acts as a secure proxy, allowing clients to communicate with the `volary` inside microVMs without ever needing to know their private IP addresses.

## The "It Just Works" Networking

The control plane is responsible for Volant's magical, zero-configuration networking. The `volar setup` command configures the host with a private bridge (`vbr0`), and `volantd` manages a static IP pool for this bridge.

This vertically integrated approach removes the single biggest point of failure and complexity in most virtualization platforms.