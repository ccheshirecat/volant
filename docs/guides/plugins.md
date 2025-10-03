---
title: "Using Plugins"
description: "Extending the engine with runtime manifests and external runtimes."
---

# Plugins & Runtime Manifests

Volant treats every specialized workload as a **plugin**. A plugin is defined by a manifest that declares:

- Optional runtime identifier (used for metadata only; defaults to the plugin name when omitted)
- Artifact references (root filesystem bundle, optional OCI images, checksums/signatures)
- Resource hints (CPU, memory)
- Workload contract (entrypoint, HTTP/WebSocket base URLs, environment variables)
- Optional action helpers or OpenAPI references for discovery

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
  "name": "steel-browser",
  "version": "0.1.0",
  "runtime": "steel",
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 2048
  },
  "workload": {
    "type": "http",
    "entrypoint": [
      "/app/api/entrypoint.sh",
      "--no-nginx"
    ],
    "base_url": "http://127.0.0.1:3000",
    "env": {
      "DOMAIN": "127.0.0.1:3000",
      "CDP_DOMAIN": "127.0.0.1:9223"
    }
  },
  "openapi": "https://raw.githubusercontent.com/steel-dev/steel-browser/main/api/openapi/schemas.json",
  "enabled": true,
  "labels": {
    "tier": "reference"
  }
}
```

If you omit the `runtime` field, Volant automatically treats the plugin `name` as its runtime identifier.

The engine never interprets plugin-specific payloads; it launches the declared workload and lets clients discover capabilities through the manifest (e.g., via the OpenAPI spec). If a plugin wants to publish convenience “actions”, it can, but they are optional.

---

## Plugin lifecycle

1. **Install** — `volar plugins install --manifest ...` registers the manifest, validates it against the schema, persists it, and adds it to the in-memory registry.
2. **Enable** — only enabled manifests can be targeted when creating VMs. (New installs default to enabled.)
3. **Launch** — when creating a VM, provide `--plugin=<name>`; the orchestrator encodes the manifest payload and injects it via kernel cmdline.
4. **Interaction** — clients consult the manifest (and its published OpenAPI schema, if present) to call the workload directly. Legacy `/actions` proxies remain for backwards compatibility but are no longer required.

---

## Plugin repositories

The engine repository focuses on orchestration primitives. Runtime implementations, initramfs builds, and user-facing workflows belong in separate plugin repositories. Those repos typically provide:

- The manifest (`plugin.yaml`/`plugin.json`)
- Runtime assets (initramfs, kernel overlay, agent extensions)
- Their own CLI/TUI or documentation for workload-specific endpoints/openAPI schemas

The engine stays stable and universal; plugins can iterate independently.

---

## Authoring guides

- Define a manifest structure and validate with the engine’s JSON schema (forthcoming).
- Package runtime assets and expose any agent routes required.
- Publish installation instructions referencing `volar plugins install`.

(Comprehensive plugin author documentation will live in the plugin toolkit repository.)

---

## Examples and Schema

- Working example manifest (nginx):
  - https://raw.githubusercontent.com/volantvm/volant/main/docs/examples/plugins/nginx.manifest.json

- JSON Schema for validation:
  - https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json
