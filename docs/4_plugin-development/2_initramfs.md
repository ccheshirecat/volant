 
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
# BusyBox defaults are applied automatically; override if you need a different build
# busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
# busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"

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
# BusyBox defaults will be injected if omitted

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
# BusyBox defaults will be injected if omitted

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

## Overlay from a Dockerfile

 Fledge can execute Dockerfiles inside its embedded BuildKit and merge the resulting filesystem into the initramfs before adding your init payload.

### Config-driven workflow

 ```toml
 version = "1"
 strategy = "initramfs"

 [agent]
 source_strategy = "release"
 version = "latest"

 [source]
 dockerfile = "./Dockerfile"
 context = "."
 target = "final"          # optional multi-stage target
 build_args = { APP_VERSION = "1.0.0" }

 [mappings]
 "./extra-config" = "/etc/myapp/config.toml"
 ```

 Fledge runs the Dockerfile via the embedded BuildKit solver, overlays the result, applies mappings, and injects the agent according to the chosen init mode.

### Direct CLI workflow

 Skip the config file entirely by pointing `fledge build` at a Dockerfile:

 ```bash
 sudo fledge build ./Dockerfile \
   --context . \
   --build-arg APP_VERSION=1.0.0 \
   --output-initramfs \
   --output myapp-initramfs
 # → outputs myapp-initramfs.cpio.gz + myapp-initramfs.manifest.json
 ```

 `--output-initramfs` switches the artifact suffix to `.cpio.gz`. Combine it with `--target`, repeatable `--build-arg`, and `--output` to customize the build.

## Additional references

- BusyBox defaults and agent validation: fledge/internal/config/config.go
- Build pipeline: fledge/internal/builder/initramfs.go
- Init wrapper behavior: fledge/internal/builder/embed/init.c
- Manifest schema: docs/schemas/plugin-manifest-v1.json
