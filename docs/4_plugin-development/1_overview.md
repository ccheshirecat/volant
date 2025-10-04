# Plugin Development Overview

A comprehensive guide to creating Volant plugins with **fledge**.

---

## What is a Plugin?

A **Volant plugin** is a self-contained, bootable artifact that contains:

1. **Your application** (binary, scripts, or OCI image)
2. **kestrel agent** (Volant's PID 1 supervisor)
3. **Plugin manifest** (JSON metadata describing the workload)
4. **Dependencies** (libraries, configuration files, etc.)

Plugins come in two forms:

- **Initramfs** (`.cpio.gz`) — Lightweight, RAM-based, fast boot
- **Rootfs** (`.img`) — Full filesystem from Docker/OCI images

---

## The Plugin Development Workflow

```
┌──────────────┐    ┌──────────┐    ┌─────────────┐    ┌──────────┐
│ Application  │ →  │  fledge  │ →  │   Plugin    │ →  │  Deploy  │
│   Source     │    │  build   │    │  Artifacts  │    │ to Volant│
└──────────────┘    └──────────┘    └─────────────┘    └──────────┘
                                           ↓
                              ┌────────────┴─────────────┐
                              │                          │
                         manifest.json            plugin.cpio.gz
                         (metadata)                (bootable)
```

**Steps**:

1. **Choose a strategy**: Initramfs or OCI Rootfs
2. **Create `fledge.toml`**: Declare your build configuration
3. **Run `fledge build`**: Generate plugin artifacts
4. **Install in Volant**: `volar plugins install --manifest <file>`
5. **Deploy**: `volar vms create <name> --plugin <plugin>`

---

## Build Strategies Compared

| Feature | Initramfs | OCI Rootfs |
|---------|-----------|------------|
| **Source** | Custom binaries | Docker/OCI images |
| **Boot Time** | 50-150ms | 2-5s |
| **Size** | 5-50 MB | 50 MB - 2 GB |
| **Persistence** | RAM-only (ephemeral) | Disk-backed |
| **Use Case** | Stateless apps, edge | Full apps, databases |
| **Complexity** | Simple | Moderate |
| **Dependencies** | Manual (static binaries) | Automatic (from image) |

---

## Quick Start: Two Paths

### Path 1: From a Docker Image (OCI Rootfs)

**Best for**: Existing Docker images, complex dependencies

```bash
mkdir my-plugin && cd my-plugin

cat > fledge.toml <<EOF
version = "1"
strategy = "oci_rootfs"

[source]
image = "nginx:alpine"

[filesystem]
type = "ext4"
size_buffer_mb = 100
EOF

sudo fledge build

# Output: nginx.img + manifest.json
volar plugins install --manifest manifest.json
```

### Path 2: Custom Binary (Initramfs)

**Best for**: Stateless services, minimal footprint, fast boot

```bash
mkdir my-plugin && cd my-plugin

# Compile your app (must be static)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o myapp

cat > fledge.toml <<EOF
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"

[mappings]
"myapp" = "/usr/bin/myapp"
EOF

sudo fledge build

# Output: plugin.cpio.gz + manifest.json
volar plugins install --manifest manifest.json
```

---

## The fledge.toml Configuration File

Every plugin is defined by a `fledge.toml` file:

### Minimal Initramfs Example

```toml
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"
```

### Minimal OCI Rootfs Example

```toml
version = "1"
strategy = "oci_rootfs"

[source]
image = "nginx:alpine"

[filesystem]
type = "ext4"
size_buffer_mb = 50
```

### With Custom Files

```toml
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"

[mappings]
"./app/binary" = "/usr/bin/myapp"
"./configs" = "/etc/myapp"
"./scripts/startup.sh" = "/usr/local/bin/startup.sh"
```

---

## Plugin Manifest

The **manifest.json** file describes your plugin to Volant:

```json
{
  "schema_version": "1.0",
  "name": "nginx",
  "version": "1.0.0",
  "runtime": "nginx",
  "enabled": true,

  "initramfs": {
    "url": "/path/to/plugin.cpio.gz",
    "checksum": "sha256:abc123..."
  },

  "resources": {
    "cpu_cores": 2,
    "memory_mb": 1024
  },

  "workload": {
    "type": "http",
    "entrypoint": ["/usr/sbin/nginx", "-g", "daemon off;"],
    "base_url": "http://127.0.0.1:80",
    "env": {
      "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    }
  },

  "health_check": {
    "endpoint": "/",
    "timeout_ms": 10000
  }
}
```

**Key fields**:

- **`name`**: Unique plugin identifier
- **`runtime`**: Runtime identifier (typically same as name)
- **`initramfs` or `rootfs`**: Boot media location and checksum
- **`resources`**: Default CPU and memory allocations
- **`workload`**: How to start and interact with your app
- **`health_check`**: How Volant verifies the app is running

---

## Building Your Plugin

### Build Command

```bash
sudo fledge build [options]
```

**Common options**:

- `-v, --verbose` — Show detailed build steps
- `-o, --output <file>` — Specify output filename
- `--config <file>` — Use alternative config file (default: `fledge.toml`)

### Build Process

**For Initramfs**:

1. Download busybox and verify checksum
2. Download kestrel agent (from GitHub or local)
3. Compile C init shim
4. Create FHS directory structure
5. Apply custom file mappings
6. Generate CPIO archive
7. Compress with gzip
8. Normalize timestamps for reproducibility
9. Generate manifest.json

**For OCI Rootfs**:

1. Pull OCI image (via skopeo)
2. Unpack layers (via umoci)
3. Install kestrel agent into rootfs
4. Apply custom file mappings
5. Create filesystem image (ext4/xfs/btrfs)
6. Mount via loop device
7. Copy files
8. Shrink to minimal size (ext4 only)
9. Generate manifest.json

---

## File Mappings

Custom files are mapped using the `[mappings]` section:

```toml
[mappings]
"source/file" = "/destination/path"
```

**Rules**:

1. **Source paths** are relative to `fledge.toml`
2. **Destination paths** must be absolute
3. **Directories** are copied recursively
4. **Permissions** are set automatically:
   - `/usr/bin/*`, `/bin/*`, `/usr/local/bin/*` → `755` (executable)
   - `/lib/*`, `/usr/lib/*` → `755`
   - Everything else → `644` (files) or `755` (directories)

**Examples**:

```toml
[mappings]
# Single binary
"myapp" = "/usr/bin/myapp"

# Configuration file
"config.yaml" = "/etc/myapp/config.yaml"

# Entire directory
"static/" = "/var/www/static"

# Script with custom location
"scripts/startup.sh" = "/usr/local/bin/startup.sh"

# Library
"libcustom.so" = "/usr/lib/libcustom.so"
```

---

## Static Compilation

For **initramfs plugins**, binaries must be **statically linked** (no dynamic dependencies).

### Go

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o myapp
```

### Rust

```bash
cargo build --release --target x86_64-unknown-linux-musl
```

### C/C++

```bash
gcc -static -o myapp myapp.c
# or
g++ -static -o myapp myapp.cpp
```

**Verify static linking**:

```bash
ldd myapp
# Should output: "not a dynamic executable"
```

---

## Testing Your Plugin

### 1. Quick Test with Volant

```bash
# Install plugin
volar plugins install --manifest manifest.json

# Create VM
volar vms create test --plugin my-plugin --cpu 2 --memory 512

# Check status
volar vms list

# View logs
volar vms logs test --follow

# Test your app
curl http://192.168.127.100
```

### 2. Debug Build Issues

```bash
# Verbose output
sudo fledge build -v

# Check artifact
ls -lh plugin.cpio.gz

# Inspect initramfs contents
zcat plugin.cpio.gz | cpio -t

# For rootfs
sudo mount -o loop myapp.img /mnt
ls -R /mnt
sudo umount /mnt
```

---

## Common Patterns

### Pattern 1: Web Server

```toml
version = "1"
strategy = "oci_rootfs"

[source]
image = "nginx:alpine"

[filesystem]
type = "ext4"
size_buffer_mb = 100

[mappings]
"configs/nginx.conf" = "/etc/nginx/nginx.conf"
"html/" = "/usr/share/nginx/html"
```

### Pattern 2: API Server

```toml
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"

[mappings]
"api-server" = "/usr/bin/api-server"
"config.json" = "/etc/api/config.json"
```

### Pattern 3: Worker Process

```toml
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
busybox_sha256 = "6e123e7f3202a8c1e9b1f94d8941580a25135382b99e8d3e34fb858bba311348"

[mappings]
"worker" = "/usr/bin/worker"
".env" = "/etc/worker/.env"
```

---

## Best Practices

### Security

1.  **Always verify checksums** for downloaded artifacts
2.  **Use specific versions** in production (not `latest`)
3.  **Scan OCI images** for vulnerabilities (Trivy, Snyk)
4.  **Minimize attack surface** — remove unnecessary files
5.  **Use minimal base images** (Alpine, distroless)

### Performance

1.  **Keep initramfs small** — every MB affects boot time
2.  **Use static binaries** — no runtime linking overhead
3.  **Choose ext4** for OCI unless you need advanced features
4.  **Strip binaries** — `strip -s myapp` to reduce size

### Maintainability

1.  **Version your plugins** — match application versions
2.  **Document custom mappings** — explain non-obvious files
3.  **Store fledge.toml in git** — alongside application code
4.  **Test before deploying** — validate in staging environment

---

## Troubleshooting

### Build Fails: "must run as root"

**Solution**: Use `sudo`

```bash
sudo fledge build
```

### Build Fails: "skopeo not found"

**Solution**: Install OCI tools

```bash
# Debian/Ubuntu
sudo apt install skopeo

# macOS
brew install skopeo
```

### VM Boots But App Doesn't Start

**Check**:

1. Is binary statically linked? `ldd myapp`
2. Is entrypoint correct in manifest?
3. Are file permissions correct? (should be `755` for executables)
4. Check logs: `volar vms logs <vm-name>`

### Plugin Size Too Large

**Solutions**:

1. Use Alpine-based images instead of Ubuntu
2. Multi-stage Docker builds to exclude build tools
3. Strip debug symbols: `strip -s binary`
4. Remove unnecessary files via mappings

---

## Next Steps

- **[Building Initramfs Plugins](2_initramfs.md)** — Deep dive into custom appliances
- **[Building OCI Rootfs Plugins](3_oci-rootfs.md)** — Converting Docker images
- **[Plugin Manifest Reference](../6_reference/1_manifest-schema.md)** — Complete manifest specification
- **[Example Plugins](4_examples.md)** — Real-world plugin configurations

---

*Ready to build your first plugin? Let's go!*
