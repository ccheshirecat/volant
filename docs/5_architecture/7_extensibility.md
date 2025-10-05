# Extensibility and Conventions

## Plugins

- Manifest schema: docs/schemas/plugin-manifest-v1.json
- Authoring guides: docs/4_plugin-development/*
- Install flow: POST /api/v1/plugins with manifest; stored in DB; exposed via registry

## Config Overrides

- vmconfig.Config supports per-VM overrides for boot media, kernel cmdline, API host/port, resources, and cloud-init.
- CLI flags map to override fields; flags take precedence when provided.

## Networking

- Declarative in manifest; overridable in config; easy to add more modes by extending helpers and launcher args.

## Storage/DB

- New entities → add migrations in internal/server/db/sqlite/migrations and repository interfaces in db package.
