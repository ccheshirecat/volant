# Building Initramfs Plugins

A deep dive into creating lightweight, fast-booting initramfs plugins with **fledge**.

---

## What is Initramfs?

**Initramfs** (initial RAM filesystem) is a RAM-based root filesystem that loads into memory during boot. Unlike traditional disk-based systems, the entire filesystem lives in RAM, making it:

- **Fast** — Boot times of 50-150ms
- **Lightweight** — Typical sizes 5-50 MB
- **Ephemeral** — Changes don't persist between reboots
- **Secure** — Fresh state on every boot

---

## When to Use Initramfs

 **Perfect for**:
- Stateless HTTP services
- API gateways
- Queue workers
- Data processing pipelines
- Edge computing workloads
- CI/CD test environments

❌ **Not suitable for**:
- Databases requiring persistence
- Applications that write large amounts of data
- Workloads requiring >100 MB of dependencies

---

## Build Workflow

```
┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   fledge     │ →  │   Download   │ →  │   Assemble   │ →  │   Compress   │
│  fledge.toml │    │ Dependencies │    │  Filesystem  │    │ plugin.cpio.gz│
└──────────────┘    └──────────────┘    └──────────────┘    └──────────────┘
                           ↓                   ↓                      ↓
                      busybox            FHS structure         Normalized
                      kestrel           + your files            timestamps
```

**Steps**:
1. Fledge downloads busybox (for Unix utilities)
2. Fledge downloads/sources kestrel agent
3. Fledge compiles a minimal C init program
4. Fledge creates standard Linux directory structure
5. Your custom files are mapped into the filesystem
6. Everything is archived into CPIO format
7. Compressed with gzip
8. Timestamps normalized for reproducibility

---

## The fledge.toml Configuration

### Minimal Example

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

### With Custom Application

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
"myapp" = "/usr/bin/myapp"
"config.yaml" = "/etc/myapp/config.yaml"
```

---

## Static Compilation

**Critical requirement**: All binaries in initramfs must be **statically linked** (no dynamic library dependencies).

### Go Applications

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o myapp
```

**Verification**:
```bash
ldd myapp
# Should output: "not a dynamic executable"

file myapp
# Should include: "statically linked"
```

### Rust Applications

```bash
# Install musl target
rustup target add x86_64-unknown-linux-musl

# Build static binary
cargo build --release --target x86_64-unknown-linux-musl

# Output: target/x86_64-unknown-linux-musl/release/myapp
```

### C/C++ Applications

```bash
# GCC
gcc -static -o myapp myapp.c

# G++
g++ -static -o myapp myapp.cpp -lstdc++
```

**For complex projects with dependencies**, consider using:
- **Alpine Linux** as build environment (musl libc)
- **Docker multi-stage builds** with `FROM scratch`
- **Static linking flags**: `-static-libgcc -static-libstdc++`

---

## File Mappings

The `[mappings]` section defines which files go into your initramfs.

### Basic Mappings

```toml
[mappings]
"myapp" = "/usr/bin/myapp"
```

Source path is relative to `fledge.toml`, destination is absolute.

### Multiple Files

```toml
[mappings]
"build/myapp" = "/usr/bin/myapp"
"configs/app.yaml" = "/etc/myapp/config.yaml"
"scripts/healthcheck.sh" = "/usr/local/bin/healthcheck"
"static/logo.png" = "/var/www/static/logo.png"
```

### Directory Mappings

```toml
[mappings]
"configs/" = "/etc/myapp"
"static/" = "/var/www/static"
"scripts/" = "/usr/local/bin"
```

Directories are copied recursively.

### Permission Handling

Fledge automatically sets permissions based on path:

| Destination Path | Permission | Description |
|------------------|------------|-------------|
| `/usr/bin/*` | `755` | Executables |
| `/bin/*` | `755` | Executables |
| `/usr/local/bin/*` | `755` | Executables |
| `/lib/*` | `755` | Libraries |
| `/usr/lib/*` | `755` | Libraries |
| Everything else | `644` | Regular files |
| Directories | `755` | Traversable |

---

## Directory Structure

Fledge creates a standard FHS (Filesystem Hierarchy Standard) structure:

```
/
├── bin/              # Essential binaries (busybox symlinks)
├── sbin/             # System binaries
├── lib/              # Essential libraries
├── usr/
│   ├── bin/          # User binaries (your app goes here)
│   ├── sbin/
│   └── lib/
├── etc/              # Configuration files
├── var/
│   ├── log/          # Log files
│   └── lib/          # Application state
├── tmp/              # Temporary files
├── dev/              # Device files (created at boot)
├── proc/             # Process information (mounted at boot)
├── sys/              # System information (mounted at boot)
└── init              # PID 1 (launches kestrel)
```

---

## Busybox

**Busybox** provides essential Unix utilities (over 300 commands in a single binary).

### Why Busybox?

- **Single binary** (~800 KB) replaces hundreds of utilities
- **Static compilation** by default
- **Battle-tested** in embedded systems for decades

### Available Commands

Busybox provides:
- **File operations**: `ls`, `cp`, `mv`, `rm`, `find`, `grep`
- **Text processing**: `cat`, `sed`, `awk`, `cut`, `tr`
- **Networking**: `wget`, `nc`, `ifconfig`, `route`
- **Shell**: `sh` (ash variant, POSIX-compliant)
- **System**: `ps`, `top`, `mount`, `chmod`, `chown`

### Custom Busybox Versions

You can use different busybox builds:

```toml
[source]
# Different architecture
busybox_url = "https://busybox.net/downloads/binaries/1.35.0-aarch64-linux-musl/busybox"
busybox_sha256 = "<checksum>"

# Or your own build
busybox_url = "https://your-server.com/custom-busybox"
busybox_sha256 = "<checksum>"
```

---

## The Kestrel Agent

**Kestrel** is Volant's guest agent that runs as PID 1 inside the VM.

### Responsibilities

1. **Process management** — Starts and monitors your workload
2. **Health checks** — Monitors application health
3. **Vsock communication** — Communicates with volantd on the host
4. **Logging** — Captures stdout/stderr and forwards to host
5. **Lifecycle** — Handles shutdown signals gracefully

### Agent Sourcing Strategies

#### 1. GitHub Release (Recommended)

```toml
[agent]
source_strategy = "release"
version = "latest"  # or "v0.2.0" for specific version
```

Fledge downloads from `github.com/volantvm/volant/releases`.

#### 2. Local Binary

```toml
[agent]
source_strategy = "local"
path = "./kestrel"  # or "/absolute/path/to/kestrel"
```

Useful for:
- Development/testing
- Custom agent builds
- Offline environments

#### 3. HTTP URL

```toml
[agent]
source_strategy = "http"
url = "https://your-cdn.com/kestrel-v0.2.0"
checksum = "sha256:abc123..."
```

For custom distribution or CI/CD.

---

## Build Process Internals

When you run `sudo fledge build`:

### 1. Download Phase

```
Downloading busybox...
Verifying checksum...
Downloading kestrel agent...
```

### 2. Compilation Phase

```
Compiling init program...
```

Fledge compiles a minimal C init that:
- Mounts essential filesystems (`/proc`, `/sys`, `/dev`)
- Execs kestrel as PID 1

### 3. Assembly Phase

```
Creating filesystem structure...
Installing busybox...
Installing kestrel...
Applying file mappings...
```

Creates FHS structure and copies all files.

### 4. Packaging Phase

```
Creating CPIO archive...
Compressing with gzip...
Normalizing timestamps...
```

Output: `plugin.cpio.gz`

---

## Real-World Examples

### Example 1: Go HTTP Server

**Application** (`main.go`):
```go
package main

import (
    "fmt"
    "log"
    "net/http"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello from Volant!\n")
    })

    log.Println("Starting server on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

**Build**:
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o myserver
```

**fledge.toml**:
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
"myserver" = "/usr/bin/myserver"
```

**Build plugin**:
```bash
sudo fledge build
```

---

### Example 2: Rust API with Configuration

**Application** (`src/main.rs`):
```rust
use actix_web::{web, App, HttpServer};
use serde::Deserialize;

#[derive(Deserialize)]
struct Config {
    port: u16,
    workers: usize,
}

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    let config_str = std::fs::read_to_string("/etc/api/config.toml")?;
    let config: Config = toml::from_str(&config_str)?;

    HttpServer::new(|| {
        App::new()
            .route("/", web::get().to(|| async { "API Running" }))
    })
    .workers(config.workers)
    .bind(("0.0.0.0", config.port))?
    .run()
    .await
}
```

**Build**:
```bash
cargo build --release --target x86_64-unknown-linux-musl
```

**fledge.toml**:
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
"target/x86_64-unknown-linux-musl/release/api" = "/usr/bin/api"
"config.toml" = "/etc/api/config.toml"
```

---

### Example 3: Script-Based Worker

**Application** (`worker.sh`):
```bash
#!/bin/sh
while true; do
    echo "Processing batch at $(date)"
    # Do work here
    sleep 60
done
```

**fledge.toml**:
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
"worker.sh" = "/usr/bin/worker"
```

---

## Debugging Initramfs Plugins

### Inspect Contents

```bash
# List all files
zcat plugin.cpio.gz | cpio -t

# Extract to inspect
mkdir inspect
cd inspect
zcat ../plugin.cpio.gz | cpio -id

# Examine structure
tree
```

### Check Binary Dependencies

```bash
# Should say "not a dynamic executable"
ldd myapp

# Check what's linked
file myapp
readelf -d myapp
```

### Test Locally with QEMU

```bash
# Boot with QEMU (requires vmlinux kernel)
qemu-system-x86_64 \
  -kernel /path/to/vmlinux \
  -initrd plugin.cpio.gz \
  -nographic \
  -append "console=ttyS0"
```

---

## Performance Optimization

### 1. Minimize Binary Size

```bash
# Strip debug symbols
strip -s myapp

# Use build optimization flags
go build -ldflags="-s -w" -o myapp  # Go
cargo build --release              # Rust (already optimized)
```

### 2. Compression

Fledge uses gzip by default. For smaller initramfs:

```bash
# Build then recompress with xz (slower boot, smaller size)
zcat plugin.cpio.gz | xz -9 > plugin.cpio.xz
```

### 3. Remove Unnecessary Files

Only include files your application actually needs.

---

## Common Patterns

### Pattern 1: Configuration from Environment

Instead of config files, use environment variables passed via manifest:

```toml
# In plugin manifest (manifest.json)
"workload": {
  "entrypoint": ["/usr/bin/myapp"],
  "env": {
    "PORT": "8080",
    "LOG_LEVEL": "info"
  }
}
```

### Pattern 2: Health Check Endpoint

Always include a health check endpoint:

```go
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
})
```

### Pattern 3: Graceful Shutdown

Handle SIGTERM for graceful shutdown:

```go
func main() {
    srv := &http.Server{Addr: ":8080"}

    go func() {
        if err := srv.ListenAndServe(); err != nil {
            log.Fatal(err)
        }
    }()

    // Handle shutdown
    sigint := make(chan os.Signal, 1)
    signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
    <-sigint

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    srv.Shutdown(ctx)
}
```

---

## Troubleshooting

### "exec format error"

**Cause**: Binary not statically linked or wrong architecture

**Solution**:
```bash
# Check architecture
file myapp
# Should show: ELF 64-bit LSB executable, x86-64, statically linked

# Rebuild with correct flags
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build
```

### "shared library not found"

**Cause**: Dynamic linking

**Solution**: Rebuild with static linking

### Plugin Size Too Large

**Solutions**:
1. Strip binaries: `strip -s myapp`
2. Use compiler optimization flags
3. Remove debug symbols
4. Consider splitting into multiple plugins

---

## Best Practices

1.  **Always verify static linking** before building plugin
2.  **Keep initramfs under 50 MB** for optimal performance
3.  **Use specific busybox versions** with checksums
4.  **Include health check endpoints** in your application
5.  **Test locally** before deploying to Volant
6.  **Version your plugins** (include version in artifact name)
7.  **Document environment variables** required by your app

---

## Next Steps

- **[OCI Rootfs Plugins](3_oci-rootfs.md)** — Converting Docker images
- **[Plugin Examples](4_examples.md)** — More real-world examples
- **[Manifest Schema](../6_reference/1_manifest-schema.md)** — Complete manifest reference

---

*Build fast, secure, and efficient plugins with initramfs.*
