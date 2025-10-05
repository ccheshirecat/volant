 # fledge.toml Reference

 Source of truth: fledge/internal/config/schema.go, config.go

 ```toml
 version   = "1"                 # required
 strategy  = "initramfs" | "oci_rootfs"  # required

 [agent]                          # required for initramfs default mode only
 source_strategy = "release" | "local" | "http"
 version = "latest"              # for release
 path    = "/path/to/kestrel"   # for local
 url     = "https://.../kestrel" # for http (optional checksum recommended)
 checksum = "sha256:..."         # optional but recommended (http)

 [init]                           # initramfs only; choose one mode
 path = "/init"                 # custom init mode (disables [agent])
 none = true                     # no-wrapper mode (disables [agent])

 [source]
 # initramfs
 busybox_url = "https://.../busybox"  # required for initramfs
 busybox_sha256 = "..."               # optional

 # oci_rootfs
 image = "nginx:alpine"               # required for oci_rootfs

 [filesystem]                    # oci_rootfs only
 type = "ext4"                   # ext4|xfs|btrfs
 size_buffer_mb = 100            # non-negative; default 100
 preallocate = false             # sparse (dd) if false; fallocate if true

 [mappings]                      # optional, both strategies
 "./local" = "/usr/bin/local"     # absolute dest path, FHS‑aware modes
 ```

 Validation rules (enforced in config.Validate):
 - version must equal "1"
 - strategy must be "initramfs" or "oci_rootfs"
 - initramfs: source.busybox_url is required
 - initramfs default mode: [agent] required; custom/none modes: [agent] forbidden
 - oci_rootfs: [filesystem] required; type must be one of ext4,xfs,btrfs; size_buffer_mb >= 0
 - mappings: destination must be absolute; no ".." segments

 Derived defaults (applyDefaults):
 - initramfs default mode: [agent] defaults to {source_strategy=release, version=latest}
 - oci_rootfs: [filesystem] defaults to {type=ext4, size_buffer_mb=100, preallocate=false}

 Build commands (from CLI):
 - sudo fledge build -c fledge.toml -o output.(cpio.gz|img)
