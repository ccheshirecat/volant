 # Plugin Authoring: OCI Rootfs

 Run existing Docker/OCI images as microVMs with a read/write root filesystem.

 Ground truth:
 - fledge/internal/config/schema.go (FilesystemConfig, SourceConfig)
 - fledge/internal/builder/oci_rootfs.go (build pipeline)

 ## When to Use
 - You already have a mature container image
 - You need package managers, dynamic linking, or larger dependency trees

 ## Minimal Config
 ```toml
 version = "1"
 strategy = "oci_rootfs"

 [source]
 image = "nginx:alpine"

 [filesystem]
 type = "ext4"        # ext4|xfs|btrfs
 size_buffer_mb = 100  # extra free space on top of extracted rootfs
 preallocate = false   # use sparse file (dd) or preallocate (fallocate)
 ```

 Build:
 ```bash
 sudo fledge build
 # → outputs <name>.img
 ```

 ## What Fledge Does
 - Fetch image via skopeo (docker-daemon: first, docker:// fallback)
 - Unpack layers with umoci into an intermediate rootfs
 - Optionally extract OCI config to /etc/fsify-entrypoint for introspection
 - Install kestrel agent to /bin/kestrel (when agent configured)
 - Apply file mappings and permissions following FHS
 - Create filesystem image (mkfs.ext4/xfs/btrfs), mount via loop, copy rootfs, optionally shrink (ext4)

 ## Manifest
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

 Install and run:
 ```bash
 volar plugins install --manifest nginx.manifest.json
 volar vms create web --plugin nginx
 ```

 ## File Mappings and Permissions
 - Executables under /usr/bin, /usr/sbin, /bin, /sbin -> 0755
 - Libraries under /lib, /usr/lib -> 0755
 - Others default to 0644 unless already executable

 See fledge/internal/builder/mapping.go for rules.
