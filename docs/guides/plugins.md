---
title: "Using Plugins"
description: "Extending the engine with runtime manifests and external runtimes."
---

# Plugins & Runtime Manifests

Volant treats every specialized workload as a **plugin**. A plugin is defined by a manifest that declares:

- The runtime identifier (e.g., `browser`, `worker`, `inference`)
- Artifact references (root filesystem bundle, optional OCI images, checksums/signatures)
- Resource hints (CPU, memory)
- Workload contract (HTTP base URL, entrypoint, environment)
- Action definitions exposed via the agent
- Optional OpenAPI references or metadata for discovery

Plugins allow the engine to remain lightweight while enabling purpose-built runtimes to live in separate repositories.

---

## Managing plugins

Use the CLI to inspect and manage installed manifests:

```bash
# List installed manifests
volar plugins list

# Show manifest details
volar plugins show browser

# Install from local JSON (manifest must pass schema validation)
volar plugins install --manifest ./browser.manifest.json

# Enable / disable
volar plugins enable browser
volar plugins disable browser

# Remove a manifest
volar plugins remove browser
```

Each command interacts with the control plane: manifests are stored in SQLite and reloaded on daemon startup.

---

## Manifest structure

A manifest (`plugin.yaml`/`plugin.json`) typically looks like:

```json
{
  "name": "browser",
  "version": "1.0.0",
  "runtime": "browser",
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 2048
  },
  "workload": {
    "type": "http",
    "base_url": "http://127.0.0.1:8080"
  },
  "actions": {
    "navigate": {
      "description": "Navigate to URL",
      "method": "POST",
      "path": "/v1/browser/navigate",
      "timeout_ms": 60000
    }
  },
  "openapi": "https://example.com/browser-openapi.json",
  "enabled": true,
  "labels": {
    "tier": "reference"
  }
}
```

The engine never interprets plugin-specific payloads; it simply proxies requests to the agent inside the microVM. Plugin repositories own the runtime implementation, action handlers, and any higher-level workflows.

---

## Plugin lifecycle

1. **Install** — `volar plugins install --manifest ...` registers the manifest, validates it against the schema, persists it, and adds it to the in-memory registry.
2. **Enable** — only enabled manifests can service actions. (New installs default to enabled.)
3. **Launch** — when creating a VM, provide `--plugin=<name>` (runtime is inferred from the manifest). The orchestrator encodes the manifest payload and injects it into the VM kernel cmdline alongside runtime identifiers.
4. **Action routing** — API requests to `/api/v1/plugins/{plugin}/actions/{action}` resolve to the manifest-defined path inside the agent. CLI helpers wrap popular actions (navigate, screenshot, scrape, exec, GraphQL) using the same proxy.

---

## Plugin repositories

The engine repository focuses on orchestration primitives. Runtime implementations, initramfs builds, and user-facing workflows belong in separate plugin repositories. Those repos typically provide:

- The manifest (`plugin.yaml`/`plugin.json`)
- Runtime assets (initramfs, kernel overlay, agent extensions)
- Their own CLI/TUI or documentation for plugin-specific actions

The engine stays stable and universal; plugins can iterate independently.

---

## Authoring guides

- Define a manifest structure and validate with the engine’s JSON schema (forthcoming).
- Package runtime assets and expose any agent routes required.
- Publish installation instructions referencing `volar plugins install`.

(Comprehensive plugin author documentation will live in the plugin toolkit repository.)
