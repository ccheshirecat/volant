---
title: Your First VM
description: Launch and inspect a microVM with the Volant engine.
---

# Your First VM

This quickstart walks through provisioning a microVM with the Volant engine, inspecting its state, and cleaning it up. It assumes you have already installed the engine (`volar`, `volantd`, `volary`), run `volar setup` on the host, and installed at least one runtime manifest.

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
vms create my-first-vm --plugin browser --cpu 2 --memory 2048
```

`volantd` will resolve the `browser` plugin manifest, inject it into the VM’s kernel cmdline, lease an IP, prepare a TAP device, boot Cloud Hypervisor, and update the SQLite state. Within a few seconds the VM should show up with a `running` status.

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

Every VM runs the agent (`volary`) on port 8080 inside the microVM. Most plugins expose their own HTTP or WebSocket APIs—inspect the manifest to discover the exact endpoints:

```bash
volar plugins list
volar plugins manifest <plugin> --summary
```

With that information you can call the workload directly (for example, the Steel browser plugin exposes `/v1/sessions` and other HTTP endpoints on its base URL). Legacy `volar vms actions ...` helpers still exist for older manifests but are no longer required.

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