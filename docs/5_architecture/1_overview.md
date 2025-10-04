# Architecture Overview

A deep dive into how Volant orchestrates microVMs.

---

## The 10,000-Foot View

```
┌──────────────────────────────────────────────────────────────┐
│                         Host Machine                          │
│  ┌────────────────────────────────────────────────────────┐  │
│  │                   volantd (Control Plane)              │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐ │  │
│  │  │ SQLite   │  │   IPAM   │  │   REST   │  │  MCP   │ │  │
│  │  │ Registry │  │  Leasing │  │   API    │  │  API   │ │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └────────┘ │  │
│  └────────────────────┬───────────────────────────────────┘  │
│                       │                                       │
│  ┌────────────────────▼───────────────────────────────────┐  │
│  │           Cloud Hypervisor Orchestration               │  │
│  └────────────────────┬───────────────────────────────────┘  │
│                       │                                       │
│  ┌────────────────────▼───────────────────────────────────┐  │
│  │              Bridge Network (vbr0)                  │  │
│  │            192.168.127.0/24 + NAT                      │  │
│  └───┬────────────┬────────────┬────────────┬─────────────┘  │
│      │            │            │            │                 │
│  ┌───▼────┐   ┌───▼────┐   ┌───▼────┐   ┌───▼────┐          │
│  │  VM 1  │   │  VM 2  │   │  VM 3  │   │  VM N  │          │
│  │ .100   │   │ .101   │   │ .102   │   │ .xxx   │          │
│  │┌──────┐│   │┌──────┐│   │┌──────┐│   │┌──────┐│          │
│  ││Kernel││   ││Kernel││   ││Kernel││   ││Kernel││          │
│  │└───┬──┘│   │└───┬──┘│   │└───┬──┘│   │└───┬──┘│          │
│  │┌───▼──┐│   │┌───▼──┐│   │┌───▼──┐│   │┌───▼──┐│          │
│  ││kestrel│   ││kestrel│   ││kestrel│   ││kestrel│          │
│  │└───┬──┘│   │└───┬──┘│   │└───┬──┘│   │└───┬──┘│          │
│  │┌───▼──┐│   │┌───▼──┐│   │┌───▼──┐│   │┌───▼──┐│          │
│  ││Workload   ││Workload   ││Workload   ││Workload          │
│  │└──────┘│   │└──────┘│   │└──────┘│   │└──────┘│          │
│  └────────┘   └────────┘   └────────┘   └────────┘          │
└──────────────────────────────────────────────────────────────┘
```

---

## System Components

### volantd: The Control Plane

**Location**: Host machine, single process
**Language**: Go
**Binary**: `/usr/local/bin/volantd`
**Data**: `/var/lib/volant/volant.db` (SQLite)

**Responsibilities**:

1. **State Management**
   - Stores all plugin manifests in SQLite (`plugins` table)
   - Tracks VM lifecycle (created, running, stopped) in `vms` table
   - Manages IP lease allocations in `ip_leases` table
   - Maintains deployment state for scaling operations

2. **IP Address Management (IPAM)**
   - Allocates IPs from configured subnet (default: `192.168.127.0/24`)
   - Tracks leases with VM associations
   - Releases IPs when VMs are deleted
   - Prevents conflicts with deterministic allocation

3. **Orchestration**
   - Constructs Cloud Hypervisor command-line invocations
   - Launches microVMs as child processes
   - Monitors VM health and state
   - Handles graceful shutdown and cleanup
   - Implements deployment controllers (Kubernetes-style reconciliation)

4. **API Server**
   - **REST API** on `127.0.0.1:8080`
     - `/api/v1/plugins` - Plugin CRUD operations
     - `/api/v1/vms` - VM lifecycle management
     - `/api/v1/deployments` - Scaling and orchestration
     - `/api/v1/events` - Server-sent events stream
   - **MCP (Model Context Protocol)** for AI/automation clients
   - Authentication and authorization (planned)

5. **Event Bus**
   - Emits events for all state changes
   - Allows real-time monitoring and automation
   - Events: `vm.created`, `vm.started`, `vm.stopped`, `plugin.installed`, etc.

---

### kestrel: The In-Guest Agent

**Location**: Inside each microVM, PID 1
**Language**: Go (static binary)
**Binary**: `/bin/kestrel` or `/usr/local/bin/kestrel`

**Why "kestrel"?** A kestrel is a small, fast falcon—perfect symbolism for a lightweight, high-performance supervisor.

**Responsibilities**:

1. **PID 1 Duties**
   - Acts as init process (PID 1)
   - Reaps zombie processes
   - Handles signals (SIGTERM, SIGINT, SIGCHLD)
   - Coordinates graceful shutdown

2. **Two-Stage Boot Coordinator**
   - **Stage 1** (rootfs only): Mount `/dev/vda`, pivot, re-exec
   - **Stage 2** (all): Mount essentials, start workload
   - Detects initramfs vs. rootfs and adjusts accordingly

3. **Workload Supervision**
   - Reads manifest from kernel cmdline (`volant.manifest=<base64url+gzip>`)
   - Spawns entrypoint command with configured env/cwd
   - Monitors process group
   - Restarts on crash (optional, per manifest)

4. **HTTP API Proxy** (Optional)
   - Listens on port 8080 inside the VM
   - Forwards requests to the workload's `base_url`
   - Allows control plane to interact with workloads
   - Used for health checks and action proxying

5. **Health Checks**
   - Periodic HTTP probes to `workload.health_check.endpoint`
   - Reports status to control plane (via API or vsock)

---

### volar: The CLI

**Location**: Host machine
**Language**: Go
**Binary**: `/usr/local/bin/volar`

**Purpose**: User-facing interface to the control plane.

**Commands**:

```bash
# Setup
volar setup                  # Configure host (bridge, NAT, systemd)

# Plugin management
volar plugins list
volar plugins install --manifest <file>
volar plugins show <name>
volar plugins enable <name>
volar plugins disable <name>
volar plugins remove <name>

# VM lifecycle
volar vms create <name> --plugin <plugin> [--cpu N] [--memory MB]
volar vms list
volar vms show <name>
volar vms start <name>
volar vms stop <name>
volar vms delete <name>
volar vms logs <name> [--follow]
volar vms console <name>

# Deployments (scaling)
volar deployments create <name> --plugin <plugin> --replicas N
volar deployments scale <name> --replicas M
volar deployments list
volar deployments delete <name>
```

**How it works**:
- Sends HTTP requests to volantd's REST API
- Formats responses as human-readable tables or JSON
- Streams logs and events in real-time
- Connects to VMs via serial console for shell access

---

### fledge: The Plugin Builder

**Location**: Host machine or CI/CD
**Language**: Go
**Binary**: `/usr/local/bin/fledge`

**Purpose**: Convert applications into bootable Volant plugins.

**Strategies**:

1. **`oci_rootfs`** - Convert OCI images to disk images
   - Uses skopeo to download images
   - Uses umoci to unpack layers
   - Creates ext4/xfs/btrfs filesystems
   - Installs kestrel agent
   - Applies file mappings
   - Shrinks to optimal size

2. **`initramfs`** - Build custom appliances from scratch
   - Downloads busybox and kestrel
   - Embeds C init shim
   - Creates FHS directory structure
   - Applies file mappings with smart permissions
   - Generates reproducible CPIO+gzip archive

**Configuration**: `fledge.toml` (declarative TOML)

---

## The Dual-Kernel Strategy

Volant maintains **two kernels** to support both build strategies:

### 1. bzImage (For Rootfs)

**Type**: bzImage (compressed kernel)
**Contains**: Baked-in initramfs bootloader
**Location**: `/var/lib/volant/kernel/bzImage`
**Size**: ~10MB

**Initramfs contents**:
- `/sbin/init` - C shim
- `/bin/kestrel` - Agent binary
- `/bin/busybox` - Core utilities
- Symlinks for busybox utilities

**Boot flow**:
1. Kernel unpacks baked-in initramfs
2. C shim mounts `/proc`, `/sys`, `/dev`
3. C shim executes `/bin/kestrel`
4. kestrel detects `/dev/vda` (rootfs disk)
5. kestrel mounts, pivots, re-executes itself
6. kestrel continues as PID 1 in the new root

**Used for**: OCI images, any rootfs-based plugins

### 2. vmlinux (For Initramfs)

**Type**: vmlinux (uncompressed ELF kernel)
**Contains**: Nothing (pristine kernel)
**Location**: `/var/lib/volant/kernel/vmlinux`
**Size**: ~30MB (uncompressed)

**Boot flow**:
1. Kernel receives initramfs via `--initramfs` flag
2. Kernel unpacks it into RAM
3. Kernel executes `/sbin/init` (C shim from initramfs)
4. C shim mounts `/proc`, `/sys`, `/dev`
5. C shim executes `/bin/kestrel`
6. kestrel detects **no `/dev/vda`** (no disk)
7. kestrel skips pivot, continues directly
8. kestrel starts workload from initramfs

**Used for**: Custom initramfs appliances (high-performance path)

---

## Networking Architecture

### Bridge-Based Networking

Volant uses **Linux bridge networking** with static IP allocation—no overlay networks, no service meshes, no magic.

```
┌─────────────────────────────────────────────────────────┐
│                      Host Machine                        │
│                                                          │
│  ┌────────────────────────────────────────┐             │
│  │         vbr0 (Bridge)                │             │
│  │       192.168.127.1/24                  │             │
│  └─┬────┬────┬────┬────┬───────────────────┘             │
│    │    │    │    │    │                                 │
│  ┌─▼──┐ ┌▼──┐ ┌▼──┐ ┌▼──┐ ┌▼───────────┐                │
│  │tap0│ │tap1│ │tap2│ │tap3│ │   tapN   │                │
│  └─┬──┘ └┬──┘ └┬──┘ └┬──┘ └┬───────────┘                │
│    │     │     │     │     │                             │
│  ┌─▼─────▼─────▼─────▼─────▼────────────┐                │
│  │         iptables NAT                  │                │
│  │   (192.168.127.0/24 → Internet)       │                │
│  └────────────────┬───────────────────────┘               │
│                   │                                       │
│            ┌──────▼─────────┐                             │
│            │   eth0 / wlan0 │ → Internet                  │
│            └────────────────┘                             │
└─────────────────────────────────────────────────────────┘
```

**How it works**:

1. **Bridge Creation** (`volar setup`):
   ```bash
   ip link add vbr0 type bridge
   ip addr add 192.168.127.1/24 dev vbr0
   ip link set vbr0 up
   ```

2. **Per-VM TAP Devices**:
   - Cloud Hypervisor creates a TAP device for each VM
   - TAP device is attached to the bridge
   - VM's virtio-net interface connects to the TAP

3. **Static IP Allocation**:
   - volantd maintains an IP pool (`.100` - `.254`)
   - Each VM gets a deterministic IP based on availability
   - IP is passed via DHCP or kernel cmdline (depending on strategy)

4. **NAT for Internet Access**:
   ```bash
   iptables -t nat -A POSTROUTING -s 192.168.127.0/24 -j MASQUERADE
   iptables -A FORWARD -i vbr0 -j ACCEPT
   iptables -A FORWARD -o vbr0 -j ACCEPT
   ```

5. **No DNS/DHCP Server** (by default):
   - VMs can use Google DNS (8.8.8.8) or custom resolvers
   - Optional: Run dnsmasq on the bridge for local DNS

**Benefits**:
-  Simple: No overlay complexity
-  Debuggable: Standard Linux tools work (`ip`, `tcpdump`, `iptables`)
-  Performant: Native bridge performance
-  Predictable: Static IPs, no surprises

---

## Data Flow: VM Creation

Let's trace a complete VM creation flow:

### Step 1: User Invokes CLI

```bash
volar vms create nginx-demo --plugin nginx --cpu 2 --memory 1024
```

### Step 2: volar → volantd (REST API)

```http
POST /api/v1/vms HTTP/1.1
Host: localhost:8080
Content-Type: application/json

{
  "name": "nginx-demo",
  "plugin": "nginx",
  "cpu_cores": 2,
  "memory_mb": 1024
}
```

### Step 3: volantd Processes Request

1. **Validate plugin exists and is enabled**:
   ```sql
   SELECT * FROM plugins WHERE name = 'nginx' AND enabled = 1
   ```

2. **Allocate IP address**:
   ```sql
   SELECT ip FROM ip_leases WHERE vm_id IS NULL LIMIT 1
   -- Result: 192.168.127.100

   UPDATE ip_leases SET vm_id = 'nginx-demo' WHERE ip = '192.168.127.100'
   ```

3. **Encode manifest**:
   - Serialize manifest to JSON
   - Gzip compress
   - Base64url encode
   - Result: Compact string for kernel cmdline

4. **Construct Cloud Hypervisor command**:
   ```bash
   cloud-hypervisor \
     --cpus boot=2 \
     --memory size=1024M \
     --kernel /var/lib/volant/kernel/bzImage \
     --disk path=/var/lib/volant/plugins/nginx-rootfs.img \
     --net tap=vmtap-nginx-demo,mac=52:54:00:12:34:56 \
     --serial tty \
     --console off \
     --cmdline "console=ttyS0 volant.manifest=<encoded_manifest> volant.plugin=nginx volant.runtime=nginx volant.api_host=192.168.127.1 volant.api_port=8080"
   ```

5. **Launch VM**:
   - Execute Cloud Hypervisor as child process
   - Store PID in database
   - Emit `vm.created` event

### Step 4: VM Boots

1. **Kernel initializes**
2. **Initramfs unpacks** (from bzImage)
3. **C shim runs**
4. **kestrel starts** (Stage 1)
5. **kestrel pivots** (if rootfs)
6. **kestrel continues** (Stage 2)
7. **Workload starts** (NGINX)

### Step 5: Health Check

volantd polls `http://192.168.127.100:80/` (or via kestrel proxy).
Once healthy, VM state → `running`.

### Step 6: Response to User

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "name": "nginx-demo",
  "plugin": "nginx",
  "ip": "192.168.127.100",
  "state": "running",
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 1024
  }
}
```

volar CLI formats this as:

```
VM nginx-demo created with IP 192.168.127.100
```

---

## Database Schema

volantd uses SQLite for state management:

### plugins

| Column      | Type    | Description                    |
|-------------|---------|--------------------------------|
| name        | TEXT PK | Plugin name                    |
| version     | TEXT    | Plugin version                 |
| runtime     | TEXT    | Runtime identifier             |
| manifest    | TEXT    | Full JSON manifest             |
| enabled     | BOOL    | Whether plugin can be used     |
| created_at  | TIMESTAMP | Installation time            |

### vms

| Column      | Type    | Description                    |
|-------------|---------|--------------------------------|
| id          | TEXT PK | VM name/ID                     |
| plugin      | TEXT FK | Plugin name                    |
| ip          | TEXT    | Allocated IP address           |
| state       | TEXT    | running/stopped/etc            |
| pid         | INT     | Cloud Hypervisor PID           |
| cpu_cores   | INT     | CPU allocation                 |
| memory_mb   | INT     | Memory allocation              |
| created_at  | TIMESTAMP | Creation time                |
| started_at  | TIMESTAMP | Last start time              |

### ip_leases

| Column      | Type    | Description                    |
|-------------|---------|--------------------------------|
| ip          | TEXT PK | IP address                     |
| vm_id       | TEXT FK | Assigned VM (nullable)         |
| leased_at   | TIMESTAMP | Lease time                   |

### deployments

| Column        | Type    | Description                  |
|---------------|---------|------------------------------|
| name          | TEXT PK | Deployment name              |
| plugin        | TEXT FK | Plugin to deploy             |
| replicas      | INT     | Desired replica count        |
| current       | INT     | Current replica count        |
| created_at    | TIMESTAMP | Creation time              |

### events

| Column      | Type    | Description                    |
|-------------|---------|--------------------------------|
| id          | INT PK  | Event ID (auto-increment)      |
| type        | TEXT    | Event type (vm.created, etc)   |
| resource_id | TEXT    | Related resource               |
| data        | TEXT    | JSON payload                   |
| timestamp   | TIMESTAMP | Event time                   |

---

## Scaling and Deployments

Volant supports Kubernetes-style **Deployments** for declarative scaling:

```bash
# Create deployment config
cat > service-config.json <<EOF
{
  "plugin": "nginx",
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 1024
  }
}
EOF

# Create deployment with 5 replicas
volar deployments create my-service --config service-config.json --replicas 5
```

**What happens**:

1. volantd loads the plugin manifest automatically
2. volantd creates 5 VMs named `my-service-1` through `my-service-5`
3. Each gets its own IP
4. A reconciliation loop ensures `current_replicas == desired_replicas`
4. If a VM crashes, it's automatically recreated

**Scaling**:

```bash
volar deployments scale my-service --replicas 10
```

volantd creates 5 additional VMs (`my-service-5` through `my-service-9`).

**Downscaling**:

```bash
volar deployments scale my-service --replicas 3
```

volantd terminates VMs `my-service-3` and `my-service-4`, frees their IPs.

---

## Event Streaming

volantd exposes a **Server-Sent Events** (SSE) endpoint for real-time monitoring:

```bash
curl http://localhost:8080/api/v1/events
```

Output:

```
event: vm.created
data: {"name":"nginx-demo","plugin":"nginx","ip":"192.168.127.100"}

event: vm.started
data: {"name":"nginx-demo","state":"running"}

event: vm.healthy
data: {"name":"nginx-demo","health_check":"passed"}
```

**Use cases**:
- Monitoring dashboards
- Alerting systems
- Automation workflows
- CI/CD integration

---

## Security Model

### Isolation

- **Hardware-level**: Each VM runs in its own Cloud Hypervisor microVM with dedicated kernel
- **Network**: VMs are on an isolated bridge, can only access internet via NAT
- **Filesystem**: No shared filesystems between host and guests (by default)

### Checksum Verification

- All plugin artifacts (rootfs, initramfs) include SHA256 checksums
- volantd verifies checksums before launching VMs
- Prevents tampering and ensures integrity

### Reproducible Builds

- fledge creates deterministic artifacts (normalized mtimes, sorted CPIO entries)
- Same input → same output (bit-for-bit)
- Enables supply-chain verification

### Planned Features

- **Signed manifests**: Minisign or GPG signatures
- **RBAC**: Role-based access control for multi-tenant environments
- **Secrets management**: Inject secrets securely via kernel cmdline or vsock
- **SELinux/AppArmor**: Additional kernel-level hardening

---

## Performance Characteristics

### VM Density

- **Theoretical**: Limited by host memory and CPU cores
- **Practical**: ~50-100 VMs per host (depends on workload)
- **Overcommit**: CPU can be overcommitted, memory cannot

### Boot Times

- **Rootfs**: 2-5 seconds (includes disk mount and pivot)
- **Initramfs**: 50-150ms (RAM-only, no disk I/O)

### Memory Overhead

- **Kernel**: ~20MB per VM
- **kestrel**: ~5-10MB
- **Workload**: Application-dependent

### Network Performance

- **Bridge**: Near-native (negligible overhead)
- **Latency**: <1ms for host ↔ VM communication

---

## Next Steps

- **[Boot Process](2_boot-process.md)** — Deep dive into the two-stage boot
- **[Control Plane Internals](3_control-plane.md)** — How volantd works under the hood
- **[Security](4_security.md)** — Isolation, verification, and hardening

---

*Architecture overview complete. Ready to go deeper?*
