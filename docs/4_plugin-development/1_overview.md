 # Plugin development overview

 This section is the canonical guide for authoring Volant plugins using Fledge.
 It explains when to choose initramfs vs. OCI Rootfs, how fledge.toml maps to build outputs, and how those outputs map to a plugin manifest consumed by Volant.

 Ground truth references:
 - Fledge config: fledge/internal/config/schema.go, config.go
 - Fledge builders: fledge/internal/builder/*.go
 - Volant manifest: volant/internal/pluginspec/spec.go

 ## Choosing a Boot Strategy

 - Initramfs (fast boot, minimal, static payloads)
   - Best for small stateless services or apps that can run from RAM
   - Artifact: .cpio.gz; kernel: vmlinux (preferred)
 - OCI Rootfs (run existing Docker/OCI images)
   - Best for mature images with many dependencies
   - Artifact: .img (ext4/xfs/btrfs); kernel: bzImage

 See: 2_getting-started/2_quick-start-initramfs.md and 3_quick-start-rootfs.md

 ## fledge.toml → Artifacts

 - fledge.toml declares:
   - strategy: "initramfs" | "oci_rootfs"
   - source: busybox URL (initramfs) or image (OCI)
   - agent: where to source kestrel for default init mode
   - init: init mode (default, custom, none) for initramfs
   - filesystem: FS type and sizing for OCI rootfs
   - mappings: host files to include in the image

 - Outputs after build:
   - initramfs: plugin.cpio.gz + suggested manifest (initramfs.url)
   - oci_rootfs: <name>.img + suggested manifest (rootfs.url)

 ## Authoring a Manifest

 Volant consumes a JSON manifest with exactly one boot medium set by default.
 Required fields include name, version, runtime, resources, workload, and either initramfs or rootfs.
 See: 6_reference/1_manifest-schema.md and docs/schemas/plugin-manifest-v1.json

 ### Minimal initramfs manifest (example)
 ```json
 {
   "$schema": "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json",
   "schema_version": "1.0",
   "name": "myapp",
   "version": "0.1.0",
   "runtime": "myapp",
   "enabled": true,
   "initramfs": { "url": "/path/to/plugin.cpio.gz" },
   "resources": { "cpu_cores": 1, "memory_mb": 512 },
   "workload": {
     "type": "http",
     "entrypoint": ["/usr/bin/myapp"],
     "base_url": "http://127.0.0.1:8080"
   },
   "health_check": { "endpoint": "/", "timeout_ms": 10000 }
 }
 ```
 
 ### Minimal OCI manifest (example)
 ```json
 {
   "$schema": "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json",
   "schema_version": "1.0",
   "name": "nginx",
   "version": "0.1.0",
   "runtime": "nginx",
   "enabled": true,
   "rootfs": { "url": "/path/to/rootfs.img", "format": "ext4" },
   "resources": { "cpu_cores": 1, "memory_mb": 1024 },
   "workload": {
     "type": "http",
     "entrypoint": ["/docker-entrypoint.sh", "nginx", "-g", "daemon off;"],
     "base_url": "http://127.0.0.1:80"
   },
   "health_check": { "endpoint": "/", "timeout_ms": 10000 }
 }
 ```

 ## Init Modes (initramfs)

 Initramfs supports three modes controlled by the [init] section of fledge.toml:
 - Default: C init mounts basics, then execs /bin/kestrel (requires [agent])
 - Custom: your binary/script is PID 1 (install to /init), no kestrel
 - None: your binary is PID 1, you also mount /proc,/sys,/dev yourself

 See details: fledge/docs/init-modes.md and 4_plugin-development/2_initramfs.md

 ## File Mappings

 Use [mappings] in fledge.toml to place files inside the artifact following FHS:
 - Executables: /usr/bin, /usr/sbin, /bin, /sbin (0755)
 - Libraries: /lib, /usr/lib (0755)
 - Config: /etc
 - Data/logs: /var, /opt

 Fledge auto-assigns sensible permissions based on destination path.

 ## Validating and Installing

 - Validate manifest with the JSON Schema (docs/schemas/plugin-manifest-v1.json)
 - Install: volar plugins install --manifest /path/to/manifest.json
 - Run a VM: volar vms create demo --plugin <name>

 ## Next

 - 4_plugin-development/2_initramfs.md: end-to-end guide for initramfs authoring
 - 4_plugin-development/3_oci-rootfs.md: end-to-end guide for OCI rootfs authoring
 - 6_reference/2_fledge-toml.md: fledge.toml reference and validation rules
Continue with the detailed guides:
- docs/4_plugin-development/2_initramfs.md
- docs/4_plugin-development/3_oci-rootfs.md
