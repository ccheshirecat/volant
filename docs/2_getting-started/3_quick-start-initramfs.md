# Quick Start: Initramfs (Custom Appliances)

Build a hyper-optimized Caddy web server that boots in under 100ms and uses less than 20MB of memory.

This guide demonstrates the **initramfs path**—creating a custom appliance with only the files you need for maximum performance.

---

## What You'll Build

By the end of this guide, you'll have:
- Created a minimal Caddy web server plugin (20MB total size)
- Built it as a reproducible initramfs with fledge
- Deployed it with sub-100ms boot time
- Understood the performance path philosophy

**Time required**: 5-10 minutes
**Prerequisites**: [Installation complete](1_installation.md)

---

## Understanding the Initramfs Path

The initramfs strategy is the "performance path" for Volant. It allows you to:

1. Package only your application binary and its exact dependencies
2. Build it into a tiny CPIO archive (initramfs)
3. Boot directly from RAM—no disk I/O after kernel load
4. Achieve millisecond-level boot times

**How it works**:
- Volant uses the `vmlinux-generic` kernel (pristine, no baked-in initramfs)
- Your custom initramfs is passed via `--initramfs` flag
- At boot, the kernel unpacks your initramfs into RAM
- Kestrel runs directly from the initramfs (no pivot, no rootfs)
- Your application starts immediately

**Key benefits**:
- ⚡ **Boot time**: 50-150ms (vs. 2-5s for rootfs)
- 📦 **Size**: 5-50MB (vs. 200MB+ for OCI images)
- 🛡️ **Attack surface**: Minimal (only what you include)
- 🔁 **Reproducible**: Deterministic builds with checksums

---

## The Caddy Plugin: A Real-World Example

We'll build a Caddy web server that:
- Serves HTTP requests on port 80
- Has zero bloat (no package manager, no shell, just Caddy + busybox)
- Boots in <100ms
- Uses <20MB RAM
- Fits in a 20MB initramfs

---

## Step 1: Prepare the Payload

Create a project directory:

```bash
mkdir -p ~/caddy-plugin/{payload,manifest}
cd ~/caddy-plugin
```

### Download Caddy Binary

```bash
cd payload

# Download the latest Caddy static binary
CADDY_VERSION="2.8.4"  # Or latest
curl -L "https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/caddy_${CADDY_VERSION}_linux_amd64.tar.gz" -o caddy.tar.gz

# Extract
tar xzf caddy.tar.gz caddy
rm caddy.tar.gz

# Make executable
chmod +x caddy
```

### Create Caddyfile

Create `payload/Caddyfile`:

```Caddyfile
{
    admin off
}

:80 {
    respond "Hello from Caddy in a Volant microVM! "
}
```

**Why `admin off`?**
The Caddy admin API tries to bind to IPv6 by default, but our minimal initramfs doesn't include IPv6 support. Disabling the admin API keeps things simple.

Your payload directory should now look like:

```
payload/
├── caddy          # ~45MB static binary
└── Caddyfile      # Your config
```

---

## Step 2: Create the Fledge Configuration

Create `fledge.toml` in the project root:

```toml
version = "1"
strategy = "initramfs"

[agent]
source_strategy = "release"
version = "latest"

[source]
busybox_url = "https://github.com/shutingrz/busybox-static-binaries-fat/raw/refs/heads/main/busybox-x86_64-linux-gnu"
busybox_sha256 = "5c566ba50a12d8020a391346575534e818a1f5b6f729a53b7b241262fe1d1b4e"

[mappings]
"payload/caddy" = "/usr/bin/caddy"
"payload/Caddyfile" = "/etc/caddy/Caddyfile"
```

**What this does**:

- **`strategy = "initramfs"`**: Build a custom initramfs (not a rootfs disk image)
- **`[agent]`**: Auto-download the latest kestrel agent from GitHub releases
- **`[source]`**: Busybox provides basic utilities (sh, mount, ps, etc.)
- **`[mappings]`**: Copy your files into the initramfs:
  - `caddy` binary → `/usr/bin/caddy` (with execute permissions)
  - `Caddyfile` config → `/etc/caddy/Caddyfile`

Fledge will:
1. Download busybox and kestrel
2. Embed the C init shim
3. Create the FHS directory structure (`/bin`, `/etc`, `/usr/bin`, etc.)
4. Copy your mapped files with FHS-aware permissions
5. Generate a reproducible CPIO archive with gzip compression

---

## Step 3: Build the Plugin

Build the initramfs:

```bash
sudo fledge build -c fledge.toml -o plugin.cpio.gz
```

**Build process**:

```
⏳ Downloading agent (kestrel latest)...
 Agent sourced: /tmp/fledge-agent-xxx

⏳ Downloading busybox...
 Busybox downloaded

⏳ Compiling C init...
 Init compiled

⏳ Preparing file mappings...
  📁 payload/caddy → /usr/bin/caddy (executable)
  📄 payload/Caddyfile → /etc/caddy/Caddyfile
 Mappings prepared

⏳ Building initramfs...
 Initramfs complete: plugin.cpio.gz (20.1 MB)
```

This takes 10-30 seconds (mostly downloading).

### Verify the Build

```bash
ls -lh plugin.cpio.gz
# -rw-r--r-- 1 root root 20M Oct 4 12:34 plugin.cpio.gz

file plugin.cpio.gz
# plugin.cpio.gz: gzip compressed data, ...
```

### Inspect the Contents (Optional)

```bash
mkdir /tmp/inspect
cd /tmp/inspect
zcat ~/caddy-plugin/plugin.cpio.gz | cpio -idmv

ls -la
# You'll see:
# /bin/          (busybox symlinks)
# /usr/bin/caddy (your binary)
# /usr/local/bin/kestrel (the agent)
# /etc/caddy/Caddyfile (your config)
# /sbin/init     (C shim)
```

---

## Step 4: Create the Plugin Manifest

Create `manifest/caddy.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json",
  "schema_version": "1.0",
  "name": "caddy",
  "version": "0.1.0",
  "runtime": "caddy",
  "enabled": true,
  "initramfs": {
    "url": "/root/caddy-plugin/plugin.cpio.gz",
    "checksum": "cef98fd5798ce875631aa0c98613fe48005a892aac8fb6ee81535945ebb0aa09"
  },
  "resources": {
    "cpu_cores": 1,
    "memory_mb": 512
  },
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/bin/caddy", "run", "--config", "/etc/caddy/Caddyfile"],
    "base_url": "http://127.0.0.1:80",
    "workdir": "/",
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

**Get the correct checksum**:

```bash
sha256sum plugin.cpio.gz
```

Update the manifest's `initramfs.checksum` field with the output.

**Key differences from rootfs manifests**:
- Uses `initramfs` key instead of `rootfs`
- Points to a `.cpio.gz` file, not a `.img` file
- No `format` field needed (initramfs format is always CPIO+gzip)

---

## Step 5: Install the Plugin

Register the manifest with Volant:

```bash
volar plugins install --manifest manifest/caddy.json
```

Verify:

```bash
volar plugins list
```

Output:

```
NAME   VERSION  RUNTIME  ENABLED  STRATEGY
caddy  0.1.0    caddy    true     initramfs
```

Show details:

```bash
volar plugins show caddy
```

---

## Step 6: Create the MicroVM

Launch Caddy:

```bash
volar vms create caddy-demo --plugin caddy --cpu 1 --memory 512
```

Watch it boot (almost instant):

```
Creating VM caddy-demo...
Allocating IP: 192.168.127.100
Booting VM with initramfs strategy...
VM caddy-demo created with IP 192.168.127.100
```

Check status:

```bash
volar vms list
```

Output:

```
NAME        PLUGIN  IP               STATE    CPU  MEM    UPTIME
caddy-demo  caddy   192.168.127.100  running  1    512MB  2s
```

---

## Step 7: Test the Web Server

From your host:

```bash
curl http://192.168.127.100:80
```

Output:

```
Hello from Caddy in a Volant microVM!
```

**It works!** And it booted in under 100ms.

### Performance Metrics

Check the VM's actual memory usage:

```bash
volar vms stats caddy-demo
```

You'll see:
- **Boot time**: ~80-100ms
- **Memory RSS**: ~15-18MB (excluding kernel)
- **Disk I/O**: Zero (everything is in RAM)

Compare this to a rootfs-based NGINX:
- Boot time: 2-5 seconds
- Memory RSS: ~50MB base
- Disk I/O: Required for pivot_root

The initramfs path is **20-50x faster to boot** and uses **60-70% less memory**.

---

## What Just Happened?

### Boot Process (Initramfs Path)

1. **Control plane** (volantd):
   - Reads the caddy manifest from SQLite
   - Allocates IP `192.168.127.100`
   - Encodes manifest as base64url+gzip
   - Constructs Cloud Hypervisor command:
     ```bash
     cloud-hypervisor \
       --cpus boot=1 \
       --memory size=512M \
       --kernel /var/lib/volant/kernel/vmlinux-generic \
       --initramfs /root/caddy-plugin/plugin.cpio.gz \
       --net tap=vmtap0,mac=AA:BB:CC:DD:EE:FF \
       --cmdline "volant.manifest=<encoded> ..."
     ```
   - Launches the VM

2. **Kernel boots** (vmlinux-generic):
   - Loads the initramfs from the `--initramfs` parameter
   - Unpacks it directly into RAM (`/`)
   - Runs `/sbin/init` (our C shim)

3. **C shim executes**:
   - Mounts `/proc`, `/sys`, `/dev`
   - Executes `/bin/kestrel`

4. **kestrel runs** (PID 1):
   - Reads manifest from `/proc/cmdline`
   - Detects **no `/dev/vda`** (no rootfs disk)
   - Skips the pivot logic entirely
   - Mounts essential tmpfs: `/tmp`, `/run`
   - Reads workload config from manifest
   - Spawns Caddy: `/usr/bin/caddy run --config /etc/caddy/Caddyfile`
   - Monitors the process group

5. **Workload runs**:
   - Caddy starts in <50ms
   - Binds to `0.0.0.0:80`
   - Serves requests on `192.168.127.100:80`

**Total boot time**: Kernel (30ms) + init (10ms) + kestrel (20ms) + Caddy (30ms) = **~90ms**

---

## Standardized Plugin Structure

Let's formalize the directory structure for plugin authoring:

```
caddy-plugin/
├── fledge.toml              # Fledge build configuration
├── manifest/
│   └── caddy.json           # Plugin manifest
├── payload/
│   ├── caddy                # Your application binary
│   └── Caddyfile            # Configuration files
├── plugin.cpio.gz           # Built artifact (gitignored)
└── .gitignore               # Ignore build artifacts
```

### .gitignore

Create `.gitignore`:

```gitignore
# Build artifacts
plugin.cpio.gz
*.img

# Binaries (rebuild from source/download)
payload/caddy
payload/**/bin/*

# Temporary files
/tmp/
```

**Why ignore the binary?**
Your repository should be reproducible. Instead of committing the 45MB Caddy binary, commit a script or GitHub Actions workflow that downloads it from official releases with checksum verification.

---

## GitHub Actions for Verifiable Builds

Create `.github/workflows/build-plugin.yml`:

```yaml
name: Build Caddy Plugin

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      packages: write

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Install fledge
        run: |
          curl -sSL https://install.fledge.build | bash
          echo "$HOME/.local/bin" >> $GITHUB_PATH

      - name: Download Caddy
        run: |
          cd payload
          CADDY_VERSION="2.8.4"
          curl -L "https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/caddy_${CADDY_VERSION}_linux_amd64.tar.gz" -o caddy.tar.gz
          tar xzf caddy.tar.gz caddy
          rm caddy.tar.gz
          chmod +x caddy

      - name: Build plugin
        run: |
          sudo fledge build -c fledge.toml -o plugin.cpio.gz

      - name: Generate checksum
        id: checksum
        run: |
          CHECKSUM=$(sha256sum plugin.cpio.gz | awk '{print $1}')
          echo "checksum=${CHECKSUM}" >> $GITHUB_OUTPUT
          echo "Checksum: sha256:${CHECKSUM}"

      - name: Update manifest checksum
        run: |
          sed -i "s/\"checksum\": \".*\"/\"checksum\": \"sha256:${{ steps.checksum.outputs.checksum }}\"/" manifest/caddy.json

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            plugin.cpio.gz
            manifest/caddy.json
          body: |
            ## Caddy Plugin ${{ github.ref_name }}

            **Checksum**: `sha256:${{ steps.checksum.outputs.checksum }}`

            ### Install:
            ```bash
            volar plugins install --manifest https://github.com/${{ github.repository }}/releases/download/${{ github.ref_name }}/caddy.json
            ```

            ### Verification:
            ```bash
            sha256sum plugin.cpio.gz
            # Should match: ${{ steps.checksum.outputs.checksum }}
            ```
```

**Benefits**:
- **Reproducible**: Anyone can verify the build
- **Verifiable**: Checksums are public
- **Automated**: Push a tag, get a release
- **Signed**: GitHub Actions provenance

Users can install your plugin directly from the release:

```bash
volar plugins install --manifest https://github.com/you/caddy-plugin/releases/download/v0.1.0/caddy.json
```

Volant will download the initramfs from the URL in the manifest and verify the checksum.

---

## Advanced: Custom File Permissions

Fledge automatically detects executables in FHS paths (`/bin`, `/usr/bin`, `/sbin`, etc.), but you can override this:

```toml
[mappings]
"payload/my-script.sh" = "/usr/bin/my-script"  # Auto-detected as executable
"payload/data.txt" = "/etc/my-app/data.txt"    # Auto-detected as non-executable (0644)
"payload/secret.key" = "/etc/my-app/secret.key"  # Non-executable
```

For libraries:

```toml
[mappings]
"payload/libfoo.so" = "/usr/lib/libfoo.so"  # Auto-detected as library (0755)
```

---

## Adding More Dependencies

Need shared libraries? Add them to your payload and map them:

```toml
[mappings]
"payload/caddy" = "/usr/bin/caddy"
"payload/Caddyfile" = "/etc/caddy/Caddyfile"
"payload/libc.so.6" = "/lib/libc.so.6"
"payload/ld-linux-x86-64.so.2" = "/lib64/ld-linux-x86-64.so.2"
```

**Tip**: Use `ldd` to find required libraries:

```bash
ldd payload/caddy
```

For static binaries (like Caddy), `ldd` will show "not a dynamic executable", meaning no external libraries are needed.

---

## Multi-Binary Plugins

Want to include multiple applications?

```toml
[mappings]
"payload/app1" = "/usr/bin/app1"
"payload/app2" = "/usr/bin/app2"
"payload/supervisor.sh" = "/usr/bin/supervisor"
```

Manifest entrypoint:

```json
{
  "workload": {
    "entrypoint": ["/usr/bin/supervisor"],
    "env": {
      "APP1_BIN": "/usr/bin/app1",
      "APP2_BIN": "/usr/bin/app2"
    }
  }
}
```

Your supervisor script can manage both binaries.

---

## Troubleshooting

### Plugin Won't Boot

Check volantd logs:

```bash
sudo journalctl -u volantd -f
```

Common issues:
- Initramfs not found (check path in manifest)
- Checksum mismatch (recalculate)
- Corrupt CPIO archive (rebuild with fledge)

### Application Crashes

Check VM logs:

```bash
volar vms logs caddy-demo
```

Look for:
- Missing libraries (`ldd` the binary)
- Missing configuration files
- File permission issues

### Access the Shell

```bash
volar vms shell caddy-demo
```

Inside the VM:

```bash
# Check running processes
ps aux

# Test locally
curl localhost:80

# Check file permissions
ls -la /usr/bin/caddy
ls -la /etc/caddy/Caddyfile
```

---

## Comparison: Initramfs vs. Rootfs

| Metric | Initramfs (Caddy) | Rootfs (NGINX OCI) |
|--------|-------------------|--------------------|
| **Boot time** | 80-100ms | 2-5 seconds |
| **Memory** | 15-20MB | 50-80MB |
| **Disk size** | 20MB | 200MB+ |
| **Complexity** | Low (single archive) | Medium (disk image + pivot) |
| **Flexibility** | High (full control) | High (any OCI image) |
| **Update process** | Rebuild initramfs | Rebuild disk image |
| **Best for** | Performance, serverless | Compatibility, existing apps |

---

## Next Steps

You've mastered the initramfs path! Now try:

1. **[Plugin Development: Initramfs](../4_plugin-development/3_authoring-guide-initramfs.md)** — Deep dive into custom appliances
2. **[Architecture: Boot Process](../5_architecture/2_boot-process.md)** — Understand the two-stage boot in detail
3. **[Deployment Guide](../3_guides/3_scaling-and-deployments.md)** — Scale your plugins with declarative deployments

---

## Key Takeaways

 **Initramfs is fast** — Sub-100ms boot times, minimal memory footprint
 **Minimal attack surface** — Only include what you need
 **Reproducible builds** — Fledge creates deterministic artifacts
 **Verifiable distribution** — GitHub Actions with checksums
 **No disk I/O** — Everything runs from RAM

The initramfs path is perfect for:
- High-performance workloads
- Serverless-style execution (snapshot/restore)
- Security-sensitive applications
- Custom appliances with minimal dependencies

Now you have both paths mastered—compatibility (rootfs) and performance (initramfs)!

---

*Initramfs path complete. You're now a Volant power user.*
