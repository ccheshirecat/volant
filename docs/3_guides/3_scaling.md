# Scaling Guide

Run workloads at scale with Volant's Kubernetes-inspired Deployments. Declaratively manage replicas, auto-scale, and build resilient services.

---

## Quick Start

```bash
# Create a deployment with 3 replicas
volar deployments create web --plugin nginx --replicas 3

# Scale up
volar deployments scale web --replicas 10

# Scale down
volar deployments scale web --replicas 2

# Delete deployment (removes all VMs)
volar deployments delete web
```

**Result**: Volant manages VM lifecycle automatically to match your desired state.

---

## Deployments Explained

A **Deployment** is a declarative way to manage a group of identical VMs.

**Think Kubernetes**, but for microVMs:

```yaml
apiVersion: volant.io/v1
kind: Deployment
metadata:
  name: web-cluster
spec:
  replicas: 5
  plugin: nginx
  resources:
    cpu: 2
    memory: 1024
```

**Key features**:
- ✅ Declarative configuration
- ✅ Automatic reconciliation
- ✅ Health-based auto-restart
- ✅ Rolling updates (future)
- ✅ Load balancer integration (future)

---

## Creating Deployments

### Basic Deployment

```bash
volar deployments create myapp \
  --plugin myapp-plugin \
  --replicas 3 \
  --cpu 2 \
  --memory 1024
```

**What happens**:
1. volantd creates 3 VMs: `myapp-0`, `myapp-1`, `myapp-2`
2. Each VM gets an IP from the pool
3. VMs start in parallel
4. volantd monitors health

### With Labels

```bash
volar deployments create api \
  --plugin flask-api \
  --replicas 5 \
  --labels env=production,tier=backend
```

**Labels** help organize and query VMs:

```bash
# List all production VMs
volar vms list --label env=production

# List all backend VMs
volar vms list --label tier=backend
```

### With Environment Variables

```bash
volar deployments create worker \
  --plugin celery-worker \
  --replicas 10 \
  --env REDIS_URL=redis://192.168.127.50:6379 \
  --env QUEUE_NAME=tasks \
  --env LOG_LEVEL=info
```

---

## Scaling Operations

### Manual Scaling

```bash
# Scale to specific count
volar deployments scale web --replicas 20

# volantd will:
# - Create 17 new VMs (web-3 through web-19)
# - Allocate IPs
# - Start VMs
# - Wait for health checks
```

**Scaling down**:

```bash
volar deployments scale web --replicas 5

# volantd will:
# - Stop VMs web-5 through web-19
# - Release their IPs
# - Clean up resources
```

### Zero-Downtime Scaling

When scaling up, new VMs are created **before** old ones are terminated (for rolling updates):

```bash
# Rolling update (future feature)
volar deployments update web \
  --plugin nginx:v2 \
  --strategy rolling \
  --max-surge 2 \
  --max-unavailable 1
```

---

## Replica Management

### Desired vs Current State

```bash
$ volar deployments list

NAME    PLUGIN   DESIRED   CURRENT   READY   AGE
web     nginx    5         5         5       10m
api     flask    10        8         7       5m
worker  celery   20        20        20      1h
```

**States**:
- **Desired**: What you want
- **Current**: VMs that exist
- **Ready**: VMs passing health checks

**Reconciliation loop**: volantd continuously works to make `current == desired`.

### Replica Naming

VMs are named deterministically:

```
<deployment-name>-<index>
```

**Example** (`web` deployment with 3 replicas):
- `web-0`
- `web-1`
- `web-2`

**Scaling to 5**:
- `web-0` (existing)
- `web-1` (existing)
- `web-2` (existing)
- `web-3` (new)
- `web-4` (new)

**Scaling to 2**:
- `web-0` (kept)
- `web-1` (kept)
- `web-2` (terminated)

---

## Health Checks and Auto-Restart

### Health Check Configuration

**Plugin manifest** (`nginx.manifest.json`):

```json
{
  "name": "nginx",
  "version": "1.0.0",
  "workload": {
    "entrypoint": ["/usr/sbin/nginx", "-g", "daemon off;"],
    "health_check": {
      "enabled": true,
      "http": {
        "port": 80,
        "path": "/health",
        "interval_seconds": 10,
        "timeout_seconds": 5,
        "unhealthy_threshold": 3
      }
    }
  }
}
```

**How it works**:
1. volantd polls `http://<vm-ip>:80/health` every 10 seconds
2. If 3 consecutive checks fail, VM is marked unhealthy
3. volantd restarts the VM automatically
4. If restart fails, VM is recreated

### Custom Health Checks

**TCP health check**:

```json
{
  "health_check": {
    "enabled": true,
    "tcp": {
      "port": 5432,
      "interval_seconds": 15,
      "timeout_seconds": 3
    }
  }
}
```

**Process health check** (checks if PID exists):

```json
{
  "health_check": {
    "enabled": true,
    "process": {
      "name": "nginx",
      "interval_seconds": 30
    }
  }
}
```

### Disable Auto-Restart

```bash
volar deployments create web \
  --plugin nginx \
  --replicas 5 \
  --no-auto-restart
```

---

## Resource Management

### CPU and Memory Limits

```bash
volar deployments create web \
  --plugin nginx \
  --replicas 10 \
  --cpu 2 \
  --memory 512
```

**Total resources** (10 replicas × 2 CPU × 512 MB):
- **CPU**: 20 cores
- **Memory**: 5 GB

**Check host capacity**:

```bash
volar debug resources

# Output:
# Total CPU: 32 cores
# Used CPU: 20 cores (62%)
# Available CPU: 12 cores
# 
# Total Memory: 64 GB
# Used Memory: 5 GB (8%)
# Available Memory: 59 GB
```

### Resource Quotas (Future)

```yaml
# /etc/volant/quotas.yaml
quotas:
  - name: production
    max_vms: 100
    max_cpu: 64
    max_memory_gb: 128

  - name: development
    max_vms: 20
    max_cpu: 16
    max_memory_gb: 32
```

---

## Deployment Strategies

### Strategy: Recreate (Default)

**All at once**: Stop all old VMs, create all new VMs.

```bash
volar deployments update web --plugin nginx:v2 --strategy recreate
```

**Pros**:
- Simple
- Fast

**Cons**:
- Downtime during update

### Strategy: Rolling Update (Future)

**Gradual replacement**: Create new VMs, stop old VMs, one at a time.

```bash
volar deployments update web \
  --plugin nginx:v2 \
  --strategy rolling \
  --max-surge 1 \
  --max-unavailable 0
```

**Pros**:
- Zero downtime
- Gradual rollout

**Cons**:
- Slower
- Mixed versions during rollout

### Strategy: Blue-Green (Future)

**Parallel environments**: Create full new deployment, switch traffic.

```bash
volar deployments create web-blue --plugin nginx:v1 --replicas 5
volar deployments create web-green --plugin nginx:v2 --replicas 5

# Switch traffic
volar lb switch web --to web-green

# Verify, then delete old
volar deployments delete web-blue
```

**Pros**:
- Instant rollback
- Safe testing

**Cons**:
- 2× resources during transition

---

## Load Balancing

### External Load Balancer

Use HAProxy, Nginx, or Envoy to distribute traffic:

**HAProxy configuration**:

```haproxy
global
    maxconn 4096

defaults
    mode http
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

frontend web
    bind *:80
    default_backend web_cluster

backend web_cluster
    balance roundrobin
    option httpchk GET /health
    server web-0 192.168.127.100:80 check
    server web-1 192.168.127.101:80 check
    server web-2 192.168.127.102:80 check
    server web-3 192.168.127.103:80 check
    server web-4 192.168.127.104:80 check
```

**Automatic backend generation** (script):

```bash
#!/bin/bash
# generate-haproxy-backend.sh

DEPLOYMENT="web"

echo "backend ${DEPLOYMENT}_cluster"
echo "    balance roundrobin"
echo "    option httpchk GET /health"

volar vms list --deployment $DEPLOYMENT --json | jq -r '.[] | "    server \(.name) \(.ip):80 check"'
```

**Usage**:

```bash
./generate-haproxy-backend.sh >> /etc/haproxy/haproxy.cfg
sudo systemctl reload haproxy
```

### Integrated Load Balancer (Future)

```bash
volar lb create web-lb \
  --deployment web \
  --port 80 \
  --algorithm round-robin \
  --health-check /health
```

volantd automatically:
- Tracks deployment VMs
- Updates backend pool
- Handles health checks
- Exposes public endpoint

---

## Auto-Scaling (Future)

### Horizontal Pod Autoscaler (HPA)

```yaml
# web-hpa.yaml
apiVersion: volant.io/v1
kind: HorizontalPodAutoscaler
metadata:
  name: web-hpa
spec:
  deploymentRef: web
  minReplicas: 3
  maxReplicas: 20
  metrics:
    - type: cpu
      target:
        averageUtilization: 70
    - type: memory
      target:
        averageUtilization: 80
    - type: requests
      target:
        averageValue: 1000
```

**Apply**:

```bash
volar apply -f web-hpa.yaml
```

**Behavior**:
- Monitors CPU, memory, and request rate
- Scales up when thresholds exceeded
- Scales down when load decreases
- Respects min/max bounds

---

## Deployment Lifecycle

### Create

```bash
volar deployments create web --plugin nginx --replicas 5
```

### Update Plugin Version

```bash
volar deployments update web --plugin nginx:v2
```

### Change Resources

```bash
volar deployments update web --cpu 4 --memory 2048
```

### Pause/Resume

```bash
# Pause (stop all VMs, keep state)
volar deployments pause web

# Resume (restart all VMs)
volar deployments resume web
```

### Delete

```bash
# Delete deployment and all VMs
volar deployments delete web

# With confirmation
volar deployments delete web --force
```

---

## Deployment Manifest (YAML)

For declarative management:

**web-deployment.yaml**:

```yaml
apiVersion: volant.io/v1
kind: Deployment
metadata:
  name: web
  labels:
    app: nginx
    env: production
spec:
  replicas: 5
  plugin: nginx
  resources:
    cpu: 2
    memory: 1024
  networking:
    mode: bridge
  labels:
    tier: frontend
  healthCheck:
    enabled: true
    http:
      port: 80
      path: /health
      intervalSeconds: 10
```

**Apply**:

```bash
volar apply -f web-deployment.yaml
```

**Update** (change replicas in file, reapply):

```bash
# Edit yaml: replicas: 10
volar apply -f web-deployment.yaml
```

---

## Multi-Tier Applications

Deploy complete application stacks:

### Example: Web + API + Database

**Database**:

```bash
volar deployments create postgres \
  --plugin postgres:15 \
  --replicas 1 \
  --cpu 4 \
  --memory 8192 \
  --volume /var/lib/postgresql/data:persistent \
  --labels tier=database
```

**API**:

```bash
volar deployments create api \
  --plugin flask-api \
  --replicas 5 \
  --cpu 2 \
  --memory 1024 \
  --env DATABASE_URL=postgresql://postgres@192.168.127.100/mydb \
  --labels tier=backend
```

**Web Frontend**:

```bash
volar deployments create web \
  --plugin react-app \
  --replicas 3 \
  --cpu 1 \
  --memory 512 \
  --env API_URL=http://192.168.127.1:8000 \
  --labels tier=frontend
```

**Service discovery** (via DNS, future):

```bash
# API connects to database via DNS
DATABASE_URL=postgresql://postgres.volant.local/mydb

# Web connects to API via DNS
API_URL=http://api.volant.local
```

---

## Best Practices

### 1. Start Small, Scale Gradually

```bash
# Initial deployment
volar deployments create web --plugin nginx --replicas 3

# Observe performance
volar vms list --deployment web

# Scale incrementally
volar deployments scale web --replicas 5
volar deployments scale web --replicas 10
```

### 2. Use Health Checks

Always enable health checks for auto-restart:

```json
{
  "health_check": {
    "enabled": true,
    "http": {
      "port": 80,
      "path": "/health",
      "interval_seconds": 10
    }
  }
}
```

### 3. Label Everything

```bash
volar deployments create web \
  --labels app=myapp,env=prod,version=v1.0.0
```

Query by labels:

```bash
volar vms list --label env=prod
volar vms list --label app=myapp,env=prod
```

### 4. Monitor Resources

```bash
# Check resource usage
volar debug resources

# List VM resources
volar vms list --show-resources
```

### 5. Use Deployments for Stateless Workloads

**Good for deployments**:
- Web servers
- API servers
- Worker queues
- Stateless microservices

**Not ideal for deployments**:
- Databases (use single VMs with persistent volumes)
- Stateful services requiring coordination

### 6. Plan for Capacity

```
Rule of thumb:
- Leave 20% CPU headroom
- Leave 20% memory headroom
- Don't overcommit memory (VMs will fail)
```

**Example** (32-core host with 64 GB RAM):

```
Max VMs:
- CPU: 32 cores × 0.8 = 25 cores available
- If each VM uses 2 cores: 25 / 2 = 12 VMs

- Memory: 64 GB × 0.8 = 51 GB available
- If each VM uses 4 GB: 51 / 4 = 12 VMs

Safe deployment size: 12 replicas × 2 CPU × 4 GB
```

---

## Troubleshooting

### Deployment Stuck

**Problem**: Replicas not reaching desired count.

**Solutions**:

```bash
# Check deployment status
volar deployments show web

# Check individual VMs
volar vms list --deployment web

# Check logs
volar vms logs web-0

# Check resources
volar debug resources
```

**Common causes**:
- Insufficient host resources
- Plugin not installed
- Health checks failing
- Network issues

### VMs Constantly Restarting

**Problem**: VMs crash and restart in a loop.

**Solutions**:

```bash
# Check logs
volar vms logs web-0 --follow

# Check health check config
volar plugins show nginx --manifest

# Disable auto-restart temporarily
volar deployments update web --no-auto-restart

# Fix issue, re-enable
volar deployments update web --auto-restart
```

### Slow Scaling

**Problem**: Scaling operations take too long.

**Optimizations**:

1. **Use initramfs plugins** (faster boot)
2. **Pre-pull OCI images** (if using rootfs)
3. **Increase host resources** (faster parallel VM creation)
4. **Tune Cloud Hypervisor** (reduce startup overhead)

---

## Scaling Limits

### Current Limits

- **Max VMs per host**: 100 (practical limit, depends on resources)
- **Max replicas per deployment**: 100
- **Max deployments**: No hard limit

### Multi-Host Scaling (Future)

Distribute deployments across multiple hosts:

```bash
volar deployments create web \
  --plugin nginx \
  --replicas 100 \
  --multi-host
```

volantd will:
- Distribute VMs across available hosts
- Balance load evenly
- Handle host failures gracefully

---

## Next Steps

- **[Networking Guide](1_networking.md)** — Network modes for scaled deployments
- **[Monitoring Guide](4_monitoring.md)** — Track deployment health
- **[High Availability](5_high-availability.md)** — Build resilient systems

---

*Scale from 1 to 100 VMs with a single command.*
