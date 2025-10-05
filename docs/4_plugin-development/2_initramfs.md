 
 # Initramfs authoring with Fledge

 Author minimal, fast‑booting plugins using Fledge's initramfs strategy.

 Ground truth:
 - fledge/internal/config/schema.go (InitConfig, AgentConfig, SourceConfig)
 - fledge/internal/builder/initramfs.go (build pipeline)
 - fledge/internal/builder/embed/init.c (C init behavior)

 ## When to Use
 - Small static binaries, stateless services
 - Cold‑start sensitive workloads
 - Full control over PID 1 when needed

 ## Init Modes
 Fledge supports 3 modes via `[init]`:
 - Default: C init mounts /proc,/sys,/dev,/tmp,/run then execs `/bin/kestrel`. Requires `[agent]`.
 - Custom: your binary/script is PID 1. Map it to `/init`. No kestrel.
 - None: your binary is PID 1 and must mount filesystems yourself. Map to `/init`.

 See fledge/docs/init-modes.md for detailed guidance.

 ## Minimal Configs

 ### Mode 1: Default (Kestrel agent)
 ```toml
 version = "1"
 strategy = "initramfs"

 [agent]
 source_strategy = "release"
 version = "latest"

 [source]
 busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
 busybox_sha256 = "<sha256>"

 [mappings]
 "./myapp" = "/usr/bin/myapp"
 ```

 Kestrel is placed at `/bin/kestrel` and becomes PID 1 via C init.

 ### Mode 2: Custom init
 ```toml
 version = "1"
 strategy = "initramfs"

 [init]
 path = "./my-init"

 [source]
 busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"

 [mappings]
 "./my-init" = "/init"
 "./myapp" = "/usr/bin/myapp"
 ```

 Fledge copies `./my-init` to `/init` (0755). No agent allowed in this mode.

 ### Mode 3: None (your binary is PID 1)
 ```toml
 version = "1"
 strategy = "initramfs"

 [init]
 none = true

 [source]
 busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"

 [mappings]
 "./my-supervisor" = "/init"
 ```

 Your binary must mount `/proc`, `/sys`, `/dev` and handle PID 1 responsibilities.

 ## Build
 ```bash
 # Install fledge
 curl -LO https://github.com/volantvm/fledge/releases/latest/download/fledge-linux-amd64
 chmod +x fledge-linux-amd64 && sudo mv fledge-linux-amd64 /usr/local/bin/fledge

 sudo fledge build
 # → outputs plugin.cpio.gz
 ```

 ## Manifest
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

 Install and run:
 ```bash
 volar plugins install --manifest myapp.manifest.json
 volar vms create demo --plugin myapp
 ```
=======
 # Initramfs authoring with Fledge

This guide walks through building an initramfs‑based plugin and wiring it into Volant. Ground truth:
- fledge/internal/config/schema.go and config.go (init, agent, source)
- fledge/internal/builder/initramfs.go and embed/init.c
- volant/internal/pluginspec/spec.go (manifest)
- volant/internal/server/orchestrator/orchestrator.go (boot media resolution, cloud‑init seed)

## Init modes

Initramfs has three modes controlled by the [init] section:
- Default (no [init] or empty): C init mounts basics, then execs /bin/kestrel (the agent). Requires [agent].
- Custom ([init.path] set): your custom init runs as PID 1; agent is not allowed.
- None ([init.none]=true): your program becomes PID 1 directly; you must handle mounts and lifecycle yourself; agent not allowed.

The agent source is controlled via [agent]:
- source_strategy: release | local | http
- version/path/url/checksum depending on strategy

Validation rules (config.Validate):
- Default mode requires [agent]; custom/none forbid [agent].
- source.busybox_url is required.

## Minimal fledge.toml (default mode)

```toml
version  = "1"
strategy = "initramfs"

[source]
busybox_url    = "https://busybox.net/downloads/binaries/1.36.1-x86_64-linux-musl/busybox"
busybox_sha256 = "<optional sha256>"

[agent]
source_strategy = "release"
version = "latest"
```

Build with Fledge to produce an initramfs image (e.g., plugin.cpio.gz).

## Custom init mode

```toml
version  = "1"
strategy = "initramfs"

[source]
busybox_url = "https://…/busybox"

[init]
path = "./init.sh"

mappings = { "./init.sh" = "/init" }
```

No [agent] is allowed. Your /init must exec your workload and handle signals.

## None mode

```toml
version  = "1"
strategy = "initramfs"

[source]
busybox_url = "https://…/busybox"

[init]
none = true

mappings = { "./mybin" = "/init" }
```

Your binary at /init is PID 1. You must mount /proc,/sys,/dev as needed.

## Manifest wiring

Create a manifest referencing the initramfs:

```json
{
  "$schema": "./../schemas/plugin-manifest-v1.json",
  "schema_version": "1.0",
  "name": "myapp",
  "version": "0.1.0",
  "runtime": "myapp",
  "enabled": true,
  "initramfs": { "url": "https://…/plugin.cpio.gz", "checksum": "sha256:…" }
}
```

Add optional:
- cloud_init (UserData/MetaData/NetworkConfig), see docs/3_guides/2_cloud-init.md
- disks (name, source, checksum, readonly)
- devices.pci_passthrough (requires Linux host with VFIO)

## Launch and override

You can override boot media per VM via volar config:
- If config.Initramfs.URL is set, orchestrator clears RootFS; if RootFS.URL is set, it clears Initramfs.

References in code:
- Boot media resolution: orchestrator.go (CreateVM/StartVM)
- Cloud‑init seed building: cloudinit/builder.go (cloud‑localds preferred, else VFAT CIDATA)
 - Additional disks: buildAdditionalDisks()
 
