---
title: Interactive Mode (Runtime Proxy)
description: How to surface runtime-specific interactive sessions through the engine.
---

# Interactive Mode

Volant’s control plane can proxy arbitrary runtime endpoints exposed by plugins. While the engine ships without a browser runtime, the same mechanism is used by browser-focused plugins to surface Chrome DevTools, VNC views, or any custom interactive UI.

This page explains the engine-level plumbing; consult individual plugin documentation for concrete commands and payloads.

---

## How the proxy works

When a plugin exposes an interactive endpoint, it documents it in the manifest (either via an `actions` helper or by referencing an OpenAPI/WebSocket schema). The control plane simply forwards requests to the agent running inside the microVM and streams the response back to the caller without ever exposing the microVM’s private IP.

Plugins can attach helpers such as “open DevTools”, “start remote desktop”, or “expose debugger session”. The engine treats these as transparent HTTP/WebSocket proxies.

---

## CLI workflow

Legacy builds of the CLI expose a generic action surface:

```bash
# Invoke a plugin action against a VM runtime
volar vms actions <vm-name> <plugin> <action> --payload ./payload.json

# Or call unscoped plugin actions (for pooled runtimes)
volar plugins action <plugin> <action> --payload ./payload.json
```

The structure of `payload.json` and the semantics of the response are defined by the plugin. Browser plugins, for example, typically return a local URL or begin streaming a proxied DevTools session.

> Tip: Run `volar plugins manifest <name> --summary` to inspect a manifest and discover published endpoints. Many plugins now prefer publishing an OpenAPI document rather than individual action helpers.

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
- The engine proxies requests defined by the manifest. Legacy `/actions` endpoints remain but newer plugins prefer OpenAPI-documented HTTP/WebSocket interfaces.
- Use plugin manifests/docs to understand how to surface the runtime-specific UI you need.
- Browser-centric interactive commands are part of the browser plugin distribution, not the core engine.
