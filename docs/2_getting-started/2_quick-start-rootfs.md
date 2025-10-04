# Quick Start: Rootfs (OCI Images)

Run your first Docker/OCI image as a Volant microVM in 2 minutes.

This guide demonstrates the **rootfs path**—taking an existing OCI image and running it with true hardware isolation.

---

## What You'll Build

By the end of this guide, you'll have:
- Installed an NGINX plugin manifest
- Created a microVM running NGINX from an OCI image
- Accessed the web server from your host
- Learned basic VM management commands

**Time required**: 2-3 minutes
**Prerequisites**: [Installation complete](1_installation.md)

---

## Understanding the Rootfs Path

The rootfs strategy is the "easy path" for Volant. It allows you to:

1. Take any OCI image (Docker Hub, GHCR, private registry)
2. Use `fledge` to convert it to a bootable disk image
3. Boot it in a Cloud Hypervisor microVM
4. Get hardware isolation without changing your application

**How it works**:
- Volant uses the `bzImage-volant` kernel (includes baked-in initramfs bootloader)
- The OCI image is converted to a disk image (`.img` file)
- At boot, kestrel mounts `/dev/vda`, pivots into it, and starts your workload
- Your application runs in a real VM with its own kernel

---

## Step 1: Create the Plugin Manifest

First, let's create a manifest for an NGINX plugin. The manifest tells Volant everything about your plugin: what image to use, how much resources it needs, and how to start the workload.

Create a file called `nginx-plugin.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json",
  "schema_version": "1.0",
  "name": "nginx",
  "version": "1.0.0",
  "runtime": "nginx",
  "enabled": true,
  "rootfs": {
    "url": "/var/lib/volant/plugins/nginx-rootfs.img",
    "checksum": "sha256:REPLACE_WITH_ACTUAL_CHECKSUM_AFTER_BUILD",
    "format": "raw"
  },
  "resources": {
    "cpu_cores": 1,
    "memory_mb": 512
  },
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/sbin/nginx", "-g", "daemon off;"],
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

**Key fields explained**:
- `rootfs.url`: Path to the disk image (will be created by fledge)
- `rootfs.format`: Either `raw` or `qcow2` (raw is simpler and faster)
- `resources`: CPU and memory allocation for the VM
- `workload.entrypoint`: Command to start your application
- `workload.base_url`: Where your application listens
- `health_check`: How volantd verifies the app is running

---

## Step 2: Build the Rootfs Image with Fledge

Now use `fledge` to convert the NGINX OCI image to a bootable disk image.

Create `fledge.toml`:

```toml
version = "1"
strategy = "oci_rootfs"

[source]
image = "docker://docker.io/library/nginx:alpine"

[filesystem]
type = "ext4"
size_buffer_mb = 100
preallocate = false
```

Build the plugin:

```bash
sudo fledge build -c fledge.toml -o nginx-rootfs.img
```

**What happens during build**:
1. Fledge downloads the NGINX OCI image using skopeo
2. Unpacks the layers using umoci
3. Downloads the latest kestrel agent from GitHub
4. Installs kestrel into the filesystem
5. Calculates the required disk size
6. Creates an ext4 filesystem
7. Copies all files with correct permissions
8. Shrinks the filesystem to optimal size

This takes 30-60 seconds depending on your internet connection.

### Get the Checksum

```bash
sha256sum nginx-rootfs.img
```

Copy this checksum and update the manifest's `rootfs.checksum` field:

```json
"checksum": "sha256:abc123def456..."
```

### Move to Plugin Directory

```bash
sudo mkdir -p /var/lib/volant/plugins
sudo mv nginx-rootfs.img /var/lib/volant/plugins/
```

---

## Step 3: Install the Plugin

Register the manifest with Volant:

```bash
volar plugins install --manifest nginx-plugin.json
```

Verify installation:

```bash
volar plugins list
```

You should see:

```
NAME    VERSION  RUNTIME  ENABLED  STRATEGY
nginx   1.0.0    nginx    true     rootfs
```

Get detailed info:

```bash
volar plugins show nginx
```

---

## Step 4: Create a MicroVM

Now create a microVM using the NGINX plugin:

```bash
volar vms create nginx-demo --plugin nginx
```

Volant will:
1. Allocate a static IP from the pool (e.g., `192.168.127.100`)
2. Encode the manifest and inject it via kernel cmdline
3. Launch Cloud Hypervisor with:
   - `bzImage-volant` kernel
   - `nginx-rootfs.img` attached as `/dev/vda`
   - 1 CPU core, 512MB RAM (from manifest)
   - Bridge networking on `volant0`

Within 2-5 seconds, your VM will boot and NGINX will be running.

### Check VM Status

```bash
volar vms list
```

Output:

```
NAME         PLUGIN  IP               STATE    CPU  MEM    UPTIME
nginx-demo   nginx   192.168.127.100  running  1    512MB  5s
```

### View Logs

```bash
volar vms logs nginx-demo
```

You'll see the boot sequence and NGINX startup logs.

---

## Step 5: Access the Webserver

Test from the host:

```bash
curl http://192.168.127.100:80
```

You should see the NGINX welcome page:

```html
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
...
```

Try in a browser:

```bash
# On the host machine
firefox http://192.168.127.100
```

**Congratulations!** You just ran an OCI image in a hardware-isolated microVM with Volant.

---

## Step 6: VM Management

### Stop the VM

```bash
volar vms stop nginx-demo
```

This gracefully shuts down the VM and releases the IP address back to the pool.

### Restart the VM

```bash
volar vms start nginx-demo
```

The VM will get the same IP address it had before (static allocation is preserved).

### Delete the VM

```bash
volar vms delete nginx-demo
```

This permanently removes the VM. The IP is freed for reuse.

### Interactive Shell (Debug)

If you need to debug inside the VM:

```bash
volar vms shell nginx-demo
```

This opens a serial console. Press `Ctrl+]` to exit.

---

## What Just Happened?

Let's trace the full lifecycle:

### Boot Process (Rootfs Path)

1. **Control plane** (volantd):
   - Reads the nginx manifest from SQLite
   - Validates resources are available
   - Allocates IP `192.168.127.100` from the pool
   - Encodes manifest as base64url-encoded, gzipped JSON
   - Constructs Cloud Hypervisor command:
     ```bash
     cloud-hypervisor \
       --cpus boot=1 \
       --memory size=512M \
       --kernel /var/lib/volant/kernel/bzImage-volant \
       --disk path=/var/lib/volant/plugins/nginx-rootfs.img \
       --net tap=vmtap0,mac=AA:BB:CC:DD:EE:FF \
       --cmdline "volant.manifest=<encoded_manifest> volant.runtime=nginx ..."
     ```
   - Launches the VM

2. **Kernel boots** (bzImage-volant):
   - Loads the baked-in initramfs bootloader
   - Runs the C shim (init.c)
   - C shim mounts `/proc`, `/sys`, `/dev`
   - C shim executes `/bin/kestrel`

3. **kestrel Stage 1** (PID 1):
   - Reads manifest from `/proc/cmdline`
   - Detects `/dev/vda` (rootfs disk)
   - Mounts `/dev/vda` to `/mnt/volant-root`
   - Copies itself to `/mnt/volant-root/usr/local/bin/kestrel`
   - Calls `busybox switch_root /mnt/volant-root /usr/local/bin/kestrel stage2`

4. **kestrel Stage 2** (after pivot):
   - Reinforces `/proc`, `/sys`, `/dev` mounts
   - Mounts `/tmp`, `/run` as tmpfs
   - Reads workload config from manifest
   - Spawns NGINX: `/usr/sbin/nginx -g "daemon off;"`
   - Monitors the process group
   - Starts HTTP API server on port 8080 (optional proxy)

5. **Workload runs**:
   - NGINX binds to `0.0.0.0:80` inside the VM
   - VM's IP is `192.168.127.100` on the bridge
   - Host can access via `http://192.168.127.100:80`

---

## Customizing Your OCI Plugin

Want to use a different image? Just change the `fledge.toml`:

### Example: PostgreSQL

```toml
version = "1"
strategy = "oci_rootfs"

[source]
image = "docker://docker.io/library/postgres:16-alpine"

[filesystem]
type = "ext4"
size_buffer_mb = 200
```

And the manifest:

```json
{
  "name": "postgres",
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/local/bin/docker-entrypoint.sh", "postgres"],
    "base_url": "http://127.0.0.1:5432",
    "env": {
      "POSTGRES_PASSWORD": "volant",
      "PGDATA": "/var/lib/postgresql/data"
    }
  },
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 2048
  }
}
```

### Example: Custom Application

If your OCI image has your own app:

```toml
[source]
image = "docker://ghcr.io/myorg/myapp:latest"
```

Manifest:

```json
{
  "name": "myapp",
  "workload": {
    "entrypoint": ["/app/start.sh"],
    "base_url": "http://127.0.0.1:3000",
    "workdir": "/app"
  }
}
```

---

## File Mappings (Advanced)

Need to add extra files to the rootfs? Use fledge's mapping feature:

```toml
version = "1"
strategy = "oci_rootfs"

[source]
image = "docker://docker.io/library/nginx:alpine"

[mappings]
"./custom-nginx.conf" = "/etc/nginx/nginx.conf"
"./static-site/" = "/usr/share/nginx/html/"
```

This overlays your custom files on top of the OCI image.

---

## Networking Options

By default, VMs get static IPs on the bridge. You can customize this in the manifest:

### Bridged Mode (Default)

```json
{
  "network": {
    "mode": "bridged",
    "auto_assign": true
  }
}
```

### Vsock Mode (No IP Networking)

For workloads that don't need network access:

```json
{
  "network": {
    "mode": "vsock"
  }
}
```

Communication happens via vsock only (control plane ↔ agent).

---

## Resource Constraints

Adjust resources in the manifest:

```json
{
  "resources": {
    "cpu_cores": 4,
    "memory_mb": 4096
  }
}
```

Override at runtime:

```bash
volar vms create nginx-big --plugin nginx --cpu 8 --memory 8192
```

---

## Troubleshooting

### VM Won't Boot

Check volantd logs:

```bash
sudo journalctl -u volantd -f
```

Common issues:
- Rootfs image not found (check path in manifest)
- Checksum mismatch (recalculate with `sha256sum`)
- Kernel not found (check `/var/lib/volant/kernel/bzImage-volant`)

### Application Not Responding

Check VM logs:

```bash
volar vms logs nginx-demo --follow
```

Look for:
- Application startup errors
- Port binding failures
- Missing dependencies

### Can't Reach VM Over Network

Verify network setup:

```bash
# Check bridge exists
ip addr show volant0

# Check IP allocation
volar vms list

# Test connectivity
ping 192.168.127.100
```

### Shell Access for Debugging

```bash
volar vms shell nginx-demo
```

Inside the VM, you can:
- Check running processes: `ps aux`
- Test localhost: `curl localhost:80`
- Inspect logs: `cat /var/log/nginx/error.log`

---

## Next Steps

Now that you've mastered the rootfs path, try:

1. **[Quick Start: Initramfs](3_quick-start-initramfs.md)** — Build a hyper-optimized custom appliance
2. **[Plugin Guide](../3_guides/2_plugins.md)** — Learn about plugin management
3. **[Authoring Guide: Rootfs](../4_plugin-development/2_authoring-guide-rootfs.md)** — Deep dive into rootfs plugin creation
4. **[Networking Guide](../3_guides/4_networking.md)** — Advanced networking configurations

---

## Key Takeaways

 **Rootfs path is Docker-compatible** — Any OCI image can become a Volant plugin
 **True hardware isolation** — Each VM has its own kernel, not shared namespaces
 **Static IP allocation** — Predictable, debuggable networking
 **Simple tooling** — `fledge` converts images, `volar` manages VMs
 **Two-stage boot** — Kestrel handles the complex pivot dance automatically

The rootfs path is perfect for:
- Migrating existing containers to microVMs
- Running third-party applications with strong isolation
- Quick prototyping without custom builds

For maximum performance, see the initramfs path next!

---

*Rootfs path complete. Ready for the performance path?*
