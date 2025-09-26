---
title: Your First VM
description: Launch and inspect a microVM with the Volant engine.
---

# Your First VM

This quickstart walks through provisioning a microVM with the Volant engine, inspecting its state, and cleaning it up. It assumes you have already installed the engine (`volar`, `volantd`, `volary`) and run `volar setup` on the host.

---

## 1. Launch the TUI or CLI

You can operate the engine via the CLI or the full-screen TUI. To open the TUI:

```bash
volar
```

If you prefer scripting, every step below can be executed as a CLI command (`volar <command>`).

---

## 2. Create a microVM

From the TUI command input (or shell), run:

```
vms create my-first-vm --cpu 2 --memory 2048
```

`volantd` will lease an IP, prepare a TAP device, boot Cloud Hypervisor, and update the SQLite state. Within a few seconds the VM should show up with a `running` status.

---

## 3. Inspect state

Use the CLI to query details:

```bash
volar vms get my-first-vm
```

You’ll see runtime, IP, MAC, resource allocation, and process PID. To stream lifecycle events:

```bash
volar vms watch
```

---

## 4. Connect to the agent (optional)

Every VM runs the agent (`volary`) on port 8080 inside the microVM. If you have a plugin installed that exposes actions, you can proxy requests via the control plane:

```bash
volar plugins list
volar vms actions my-first-vm <plugin> <action> --payload ./payload.json
```

(See the plugin documentation for available actions; the engine ships with no runtime-specific actions enabled by default.)

---

## 5. Destroy the VM

When you’re done:

```
vms delete my-first-vm
```

`volantd` stops the Cloud Hypervisor process, cleans up TAP devices, and releases the static IP back to the pool.

---

## Next Steps

- Explore the [Control Plane guide](../guides/control-plane.md) for a deeper look at the orchestrator internals.
- Learn how to manage runtimes via [Plugins](../guides/plugins.md).
- Build engine/browser artifacts using `make build-artifacts`.