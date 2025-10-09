 
# fledge.toml reference

References:
- fledge/internal/config/schema.go
- fledge/internal/config/config.go (Validation rules are enforced in Validate())

## Editor integration

For Taplo-compatible editors (VS Code/JetBrains TOML plugins), add this comment at the very top of your fledge.toml to enable autocomplete and validation:

```
# schema = "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/fledge-toml-v1.json"
```

## Top-level

- version: string (required) — must equal "1"
- strategy: string (required) — "initramfs" or "oci_rootfs"
- agent: AgentConfig (optional; only allowed for initramfs default mode)
- init: InitConfig (optional; initramfs only)
- source: SourceConfig (required; fields depend on strategy)
- filesystem: FilesystemConfig (optional; required for oci_rootfs)
- mappings: map[string]string (optional; host→guest file placements)

## InitConfig (initramfs only)

- path: string (optional) — sets custom PID1; mutually exclusive with none=true
- none: bool (optional) — makes your binary PID1; mutually exclusive with path

Init mode is derived as:
- default: init unset or empty (requires [agent])
- custom: init.path set (forbids [agent])
- none: init.none=true (forbids [agent])

## AgentConfig (initramfs default mode only)

- source_strategy: string (required) — "release" | "local" | "http"
- version: string (required for release)
- path: string (required for local)
- url: string (required for http)
- checksum: string (optional for http)

## SourceConfig

- For oci_rootfs:
  - image: string — reference to an existing image (mutually exclusive with dockerfile)
  - dockerfile: string — path to a Dockerfile to build locally (mutually exclusive with image)
  - context: string (optional) — build context directory; defaults to the Dockerfile's directory
  - target: string (optional) — multi-stage target
  - build_args: map[string]string (optional) — forwarded as build arguments

- For initramfs:
  - dockerfile/context/target/build_args — optional Dockerfile overlay before init payload is added
  - busybox_url: string (optional) — override default BusyBox URL
  - busybox_sha256: string (optional) — override default BusyBox checksum

## FilesystemConfig (oci_rootfs only)

- type: string (required) — one of: ext4, xfs, btrfs
- size_buffer_mb: int (required) — additional free space to add; must be >= 0
- preallocate: bool (optional) — preallocate the image file

When absent, defaults are applied (DefaultFilesystemConfig):
- type: ext4
- size_buffer_mb: 100
- preallocate: false

## mappings

A map of host source path → absolute destination path inside the image. Validation:
- destination must be absolute (starts with /)
- destination cannot contain ".."

Placement rules follow FHS semantics (see fledge/internal/builder/mapping.go):
- Executables under /usr/bin, /usr/sbin, /bin, /sbin → 0755
- Libraries under /lib, /usr/lib → 0755
- Others keep mode or default to 0644

## Validation summary

- version must be "1"
- strategy must be initramfs or oci_rootfs
- initramfs:
  - default mode requires [agent]
  - custom/none forbid [agent]
  - BusyBox URL/checksum default automatically if omitted
- oci_rootfs:
  - exactly one of source.image or source.dockerfile must be set
  - [filesystem] required; type in {ext4,xfs,btrfs}; size_buffer_mb >= 0
- mappings: destination absolute and no ".."

See fledge/internal/config/config.go for full validation logic.
