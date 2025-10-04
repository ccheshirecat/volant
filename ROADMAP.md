# Volant Roadmap

A living document of where Volant is today and where it's headed.

---

## Current State (v0.1 - Foundation)

 **Core Orchestration**
- `volantd` control plane with SQLite registry
- `volar` CLI for VM/plugin management
- `kestrel` in-guest agent (PID 1)
- Cloud Hypervisor integration
- Bridge networking + IPAM

 **Plugin System**
- `fledge` builder for both strategies
- OCI rootfs conversion (NGINX, Alpine, etc.)
- Custom initramfs appliances (Caddy)
- Reproducible builds
- Plugin manifest format (JSON)

 **VM Management**
- Create/start/stop/delete lifecycle
- Configurable CPU/memory resources
- Serial console logging
- Interactive shell access
- Health checks

 **Basic Scaling**
- Kubernetes-style Deployments
- Declarative replica management
- Auto-reconciliation loops

 **Developer Experience**
- Comprehensive documentation
- Quick-start guides for both strategies
- Example plugins (NGINX, Caddy)
- Installation scripts

---

## Near-Term (v0.2-0.3 - Polish)

###  Hardening & Stability

- **Comprehensive Testing**
  - Unit tests for all components
  - Integration tests for end-to-end flows
  - Stress tests for VM density
  - Chaos testing (VM crashes, network failures)

- **Improved Error Handling**
  - Better error messages throughout
  - Graceful degradation strategies
  - Retry logic for transient failures

- **Observability**
  - Structured logging (JSON output)
  - Prometheus metrics export
  - OpenTelemetry tracing
  - Performance profiling tools

- **Security Enhancements**
  - Plugin manifest signing (minisign)
  - RBAC for multi-user environments
  - Audit logging for all control plane actions
  - SELinux/AppArmor profiles

###  User Experience

- **Web Dashboard** (Optional)
  - Real-time VM status view
  - Resource utilization graphs
  - Log streaming interface
  - Plugin marketplace

- **Cloud-Init Support**
  - Full cloud-init integration
  - User-data and meta-data services
  - Dynamic VM configuration
  - SSH key injection

- **Plugin Ecosystem**
  - Official plugin registry/marketplace
  - Verified plugins from Volant team
  - Community plugin submissions
  - Plugin search and discovery

- **Documentation Improvements**
  - Video tutorials
  - Interactive examples
  - Troubleshooting playbooks
  - FAQ section

---

## Mid-Term (v0.4-0.6 - Features)

###  Advanced Capabilities

#### **VFIO GPU Passthrough**

**Goal**: Enable GPU-accelerated workloads (ML inference, rendering, gaming).

**Timeline**: Q2-Q3 2025

**Technical Approach**:

1. **IOMMU Configuration**
   ```bash
   # Kernel cmdline: intel_iommu=on iommu=pt
   # Bind GPU to vfio-pci driver
   echo "10de 1c03" > /sys/bus/pci/drivers/vfio-pci/new_id
   ```

2. **Cloud Hypervisor Integration**
   - Add `--device` flags for PCI passthrough
   - Configure VFIO groups and device mappings
   - Handle GPU reset between VM lifecycle

3. **Plugin Manifest Extensions**
   ```json
   {
     "hardware": {
       "gpu": {
         "required": true,
         "vendor": "nvidia",
         "model": "rtx-3080",
         "vram_mb": 10240
       }
     }
   }
   ```

4. **Scheduling & Allocation**
   - volantd tracks available GPUs
   - Allocates GPUs to VMs on creation
   - Enforces GPU quotas and limits
   - Releases GPU on VM deletion

5. **Driver Management**
   - Pre-built rootfs images with NVIDIA/AMD drivers
   - CUDA/ROCm runtime bundles
   - Automatic driver version matching

**Use Cases**:
-  **ML Inference**: Serve models with NVIDIA TensorRT
-  **Cloud Gaming**: Stream games via Moonlight/Parsec
-  **Video Encoding**: FFmpeg with GPU acceleration
-  **Virtual Desktops**: GPU-accelerated VDI

**Challenges**:
- IOMMU group isolation (may require ACS override patches)
- GPU reset reliability (varies by vendor/model)
- Host driver conflicts (must unbind from host)
- Multi-tenant security (GPU memory isolation)

---

#### **PaaS Mode: App-to-Plugin Pipeline**

**Goal**: Transform Volant from an orchestrator into a full-fledged Platform-as-a-Service—users push apps, Volant builds plugins, deploys VMs, and manages traffic.

**Timeline**: Q3-Q4 2025

**Vision**:

```bash
# User workflow (Heroku-style)
git remote add volant volant@192.168.127.1:myapp
git push volant main

# Volant automatically:
# 1. Detects app type (Node.js, Python, Go, etc.)
# 2. Runs fledge to build plugin
# 3. Creates deployment with N replicas
# 4. Provisions load balancer
# 5. Returns public URL
```

**Components**:

1. **App Detection & Buildpacks**
   - Heroku-compatible buildpack support
   - Auto-detect via `package.json`, `requirements.txt`, `go.mod`, etc.
   - Generate `fledge.toml` dynamically
   - Build OCI image or initramfs based on heuristics

2. **Git Receiver**
   - Accept `git push` over SSH
   - Authenticate users with SSH keys
   - Trigger build pipeline on push
   - Stream build logs to user

3. **Build Service**
   - Queue builds (async with workers)
   - Run fledge in isolated containers
   - Cache layers for faster rebuilds
   - Store artifacts in registry

4. **Deployment Automation**
   - Create deployment with replica count
   - Blue-green or rolling updates
   - Health checks before traffic switch
   - Automatic rollback on failure

5. **Load Balancer**
   - HAProxy or Envoy as ingress
   - Route traffic to healthy VMs
   - TLS termination
   - WebSocket support

6. **Domain Management**
   - Automatic subdomain allocation (`myapp.volant.io`)
   - Custom domain support (CNAME records)
   - Wildcard SSL via Let's Encrypt
   - HTTP/2 and HTTP/3 support

7. **Secrets & Config**
   - Environment variable injection
   - Secrets stored encrypted in SQLite
   - Config changes trigger redeployments
   - Audit trail for all changes

8. **Logging & Monitoring**
   - Centralized log aggregation (Loki)
   - Application metrics (Prometheus)
   - Alerting (Alertmanager)
   - Log drains (send to external services)

**User Experience**:

```bash
# 1. Install Volant CLI
curl -fsSL https://volant.io/install | sh

# 2. Login
volant login

# 3. Create app
volant apps create myapp

# 4. Add Git remote
volant git:remote -a myapp

# 5. Deploy
git push volant main

# Output:
# -----> Detecting app type... Node.js
# -----> Installing dependencies (npm install)
# -----> Building plugin with fledge (initramfs strategy)
# -----> Deploying 3 replicas
# -----> VM myapp-0 started (192.168.127.100)
# -----> VM myapp-1 started (192.168.127.101)
# -----> VM myapp-2 started (192.168.127.102)
# -----> Provisioning load balancer
# -----> Deployed to https://myapp.volant.io

# 6. Scale
volant ps:scale web=5 -a myapp

# 7. View logs
volant logs --tail -a myapp

# 8. Run commands
volant run rake db:migrate -a myapp

# 9. Connect to shell
volant ps:exec -a myapp

# 10. Manage config
volant config:set DATABASE_URL=postgres://... -a myapp
```

**API Example**:

```json
POST /api/v1/apps
{
  "name": "myapp",
  "git_url": "https://github.com/user/myapp",
  "build_strategy": "auto",
  "scale": {
    "replicas": 3,
    "cpu": 1,
    "memory_mb": 512
  },
  "env": {
    "NODE_ENV": "production"
  }
}
```

**Challenges**:
- Build isolation (sandboxed environments)
- Build cache eviction policies
- Multi-tenancy (resource quotas, network isolation)
- Storage for large artifacts (S3-compatible backend?)
- DNS management at scale

**Use Cases**:
-  **Web Apps**: Deploy Node.js/Python/Ruby apps
-  **APIs**: Microservices architecture
-  **Background Workers**: Celery, Sidekiq, etc.
-  **Databases**: Postgres, Redis, etc. (with persistent volumes)

---

###  Networking Enhancements

- **Service Discovery**
  - DNS-based service discovery (CoreDNS)
  - VM-to-VM communication by name
  - Service meshes (Linkerd, Istio)

- **Load Balancing**
  - Integrated L4/L7 load balancer
  - Health-based traffic routing
  - Session affinity

- **Advanced Networking**
  - IPv6 support
  - Multiple network interfaces per VM
  - VLAN tagging
  - Custom routing tables

---

###  Storage

- **Persistent Volumes**
  - Attach additional block devices
  - Volume snapshots and cloning
  - Dynamic volume provisioning
  - CSI driver integration

- **Shared Storage**
  - NFS/9P for shared filesystems
  - Object storage integration (S3, MinIO)

---

## Long-Term (v1.0+ - Ecosystem)

###  Multi-Node Clustering

- Distributed control plane (etcd/Raft)
- VM migration between hosts
- Global load balancing
- Centralized monitoring and logging

###  Plugin Ecosystem

- Official plugin marketplace
- Verified publisher program
- One-click plugin installation
- Automated security scanning

###  Integrations

- Kubernetes CRI/OCI runtime
- Terraform provider
- Ansible modules
- Prometheus exporters

###  Enterprise Features

- Multi-tenancy with RBAC
- Billing and metering
- Compliance reporting (SOC 2, HIPAA)
- SLA monitoring and guarantees

---

## Research & Experiments

###  Bleeding Edge

- **WebAssembly Support**: Run Wasm modules in microVMs
- **Confidential Computing**: SEV/TDX for encrypted VMs
- **ARM64**: Full ARM support for Raspberry Pi, AWS Graviton
- **eBPF Networking**: Replace iptables with Cilium-style eBPF
- **Live Migration**: Move running VMs between hosts
- **Nested Virtualization**: VMs inside VMs (for testing)

---

## Community & Governance

### Open Source

- Apache 2.0 license
- Transparent roadmap (this document)
- Public issue tracker
- Contributor guidelines
- Code of conduct

### Releases

- **Semver**: v0.x = breaking changes expected
- **Cadence**: Minor releases every 2-3 months
- **Stability**: LTS releases once we hit v1.0

### Support

- GitHub Discussions for Q&A
- Discord/Slack for real-time chat
- Paid support tiers (future)

---

## How to Contribute

Want to help shape Volant's future?

1. **Code**: Submit PRs for features or bug fixes
2. **Docs**: Improve guides, fix typos, add examples
3. **Plugins**: Build and share your own plugins
4. **Feedback**: Open issues with feature requests or bug reports
5. **Testing**: Try Volant in production and report experiences

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## Questions?

-  Email: [email protected]
-  Discord: [discord.gg/volant](https://discord.gg/volant)
-  GitHub: [github.com/ccheshirecat/volant](https://github.com/ccheshirecat/volant)

---

*Last updated: October 2025*
