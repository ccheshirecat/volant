# Plugin Manifest Schema (v1)

This document specifies the Volant plugin manifest fields as implemented in code.

Single source of truth in code:
- File: internal/pluginspec/spec.go

Summary of required fields:
- schema_version: string
- name: string
- version: string
- runtime: string (defaults to name when empty)
- resources: { cpu_cores: int > 0, memory_mb: int > 0 }
- workload: { type: "http", base_url: string URL, entrypoint: [string, ...] }
- Exactly one of:
  - initramfs: { url: string, checksum?: string }
  - rootfs: { url: string, checksum?: string, format?: "raw"|"qcow2" }

Optional fields:
- image, image_digest (for OCI lineage)
- disks[]: { name, source, format?: raw|qcow2, checksum?, readonly, target? }
- cloud_init: { datasource, seed_mode (default vfat), user_data/meta_data/network_config }
- network: { mode: vsock|bridged|dhcp, subnet?, gateway?, auto_assign? }
- devices: { pci_passthrough?: ["0000:01:00.0"...], allowlist?: ["vendor:device" or "vendor:*"] }
- actions: map<string, { description?, method, path, timeout_ms? }>
- health_check: { endpoint, timeout_ms }
- openapi: URL or absolute file path
- labels: map<string,string>

See docs/schemas/plugin-manifest-v1.json for JSON Schema.
