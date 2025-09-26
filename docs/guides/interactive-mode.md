---
title: Interactive Mode (Runtime Proxy)
description: How to surface runtime-specific interactive sessions through the engine.
---

# Interactive Mode

Volant’s control plane can proxy arbitrary runtime endpoints exposed by plugins. While the engine ships without a browser runtime, the same mechanism is used by browser-focused plugins to surface Chrome DevTools, VNC views, or any custom interactive UI.

This page explains the engine-level plumbing; consult individual plugin documentation for concrete commands and payloads.

---

## How the proxy works

When a plugin exposes an action (via its manifest) that returns a proxied URL or starts a long-lived tunnel, the engine:

1. Resolves the manifest (`/api/v1/plugins/{plugin}/actions/{action}` or VM-scoped equivalent).
2. Forwards the request to the agent running inside the microVM.
3. Streams the response (HTTP/WebSocket/SSE) back to the caller, without ever exposing the microVM’s private IP.

Plugins can register actions such as “open DevTools”, “start remote desktop”, or “expose debugger session”. From the engine perspective, they are just HTTP proxies.

---

## CLI workflow

The engine CLI provides a generic action surface:

```bash
# Invoke a plugin action against a VM runtime
volar vms actions <vm-name> <plugin> <action> --payload ./payload.json

# Or call unscoped plugin actions (for pooled runtimes)
volar plugins action <plugin> <action> --payload ./payload.json
```

The structure of `payload.json` and the semantics of the response are defined by the plugin. Browser plugins, for example, typically return a local URL or begin streaming a proxied DevTools session.

> Tip: Run `volar plugins show <name>` to inspect a manifest and discover available actions.

---

## TUI integration

The interactive TUI lets you issue the same action commands. Select a VM, open the command input, and run:

```
plugins action <plugin> <action> --payload '{"...": ...}'
```

Any events or logs emitted by the plugin runtime will appear in the log pane thanks to the event bus.

---

## Browser-specific workflows

If you’re looking for the “open remote DevTools” or “start live browser session” commands referenced in earlier documentation, those now live in the browser plugin repository. Install the browser plugin and follow its CLI/TUI guide to enable interactive sessions.

The base engine intentionally avoids bundling browser tooling so that other runtime plugins (AI inference, workers, etc.) can coexist without assumptions.

---

## Summary

- Interactive capabilities are defined by plugins.
- The engine proxies requests via `/api/v1/plugins/.../actions/...` and the matching CLI commands.
- Use plugin manifests/docs to understand how to surface the runtime-specific UI you need.
- Browser-centric interactive commands are part of the browser plugin distribution, not the core engine.