# Cloud-init guide

Ground truth: internal/server/orchestrator/cloudinit/builder.go and orchestrator.go (prepareCloudInitSeed, mergeCloudInit).

Volant supports NoCloud datasource via a seed image attached as a read‑only disk. You can provide cloud-init documents through the plugin manifest or per‑VM config. The orchestrator merges them and builds a CIDATA image.

## Documents

- UserData (YAML cloud-config)
- MetaData (YAML with instance-id, local-hostname)
- NetworkConfig (YAML)

Each document can be provided inline (content) or via a file path (path). When a path is given, the CLI resolves relative paths based on the manifest file location and inlines the content.

## Merging rules

Effectively: override wins. The orchestrator uses mergeCloudInit(base, override):
- If override content/path set or inline=true, override replaces base for that document.
- Otherwise base is kept.

Base comes from the plugin manifest (if any). Override comes from per‑VM config.

## Seed build

The orchestrator creates a seed image at ~/.volant/run/cloudinit/<vm>-seed.img:
- If cloud-localds exists, it is used with the provided documents.
- Otherwise, a VFAT image is created with go-diskfs and the files are written manually.
- The volume label is CIDATA.

MetaData defaults are synthesized if not provided:
- instance-id: volant-<vmID>
- local-hostname: <vm name>

## Cleanup and updates

- On VM start or create, the seed is generated and attached as a read-only disk.
- On config changes, seeds are rebuilt and old seeds removed.
- On VM deletion, the seed image is removed.

## Authoring tips

- Keep cloud-config minimal for initramfs; the agent typically manages services and workload.
- For oci_rootfs, use cloud-init to write config files or manage system users/services as needed.
