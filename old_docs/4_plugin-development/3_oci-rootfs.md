# OCI Rootfs Plugin Development

This guide covers building **rootfs-based plugins** that run full-featured Linux distributions in QCOW2 or raw disk images. Unlike initramfs plugins that load entirely into RAM, rootfs plugins boot from persistent disk images with read-write filesystems.

---

## When to Use Rootfs Plugins

Choose rootfs over initramfs when you need:

- **Persistent storage** – Configuration, databases, user uploads that survive reboots
- **Package managers** – `apt`, `yum`, `apk` for installing dependencies
- **Dynamic dependencies** – Ruby gems, Python packages, Node modules installed at runtime
- **Complex applications** – Multi-process systems like WordPress, GitLab, or Jupyter
- **Larger footprint** – Applications where disk space matters more than boot speed

**Trade-offs**:
- Larger plugin artifacts (100MB–2GB typical vs 10–50MB for initramfs)
- Slightly slower boot times (disk I/O vs RAM-only)
- More complex build process (full OS installation vs static binary)

---

## Build Strategy Overview

Rootfs plugins are built using one of three approaches:

### 1. Fledge + Dockerfile (Recommended)

Build a Docker image, convert it to QCOW2 with kestrel:

```bash
# Build Docker image
docker build -t my-app .

# Convert to QCOW2 with fledge
fledge build --output plugin.tar.gz

# Install plugin
volar plugins install ./plugin.tar.gz
```

**Pros**: Familiar Docker workflow, great caching, easy multi-stage builds  
**Cons**: Requires Docker daemon

### 2. Fledge + OCI tarball

Use any OCI-compliant builder (Podman, Buildah, Kaniko) and pass the tarball to fledge:

```bash
# Build with Podman
podman build -t my-app .
podman save -o image.tar my-app

# Convert with fledge
fledge build --docker-archive image.tar --output plugin.tar.gz
```

**Pros**: No Docker daemon required, works in CI without privileged mode  
**Cons**: Extra step to export tarball

### 3. Manual QCOW2 creation

Build QCOW2 directly with `qemu-img` and `virt-install`:

```bash
# Create blank image
qemu-img create -f qcow2 rootfs.qcow2 10G

# Install OS with virt-install
virt-install --name myvm --disk rootfs.qcow2 --location http://...

# Package manually
tar czf plugin.tar.gz rootfs.qcow2 manifest.json
```

**Pros**: Full control, no container dependencies  
**Cons**: Complex, manual, hard to reproduce

---

## Quick Start: Dockerfile to Plugin

Let's build a simple NGINX web server plugin.

### Step 1: Create Project Structure

```bash
mkdir nginx-plugin && cd nginx-plugin
```

### Step 2: Write Dockerfile

```dockerfile
# nginx-plugin/Dockerfile
FROM alpine:3.19

# Install NGINX and kestrel agent
RUN apk add --no-cache nginx && \
    wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel

# Configure NGINX
COPY nginx.conf /etc/nginx/nginx.conf
COPY index.html /var/www/html/index.html

# Kestrel launches NGINX
COPY kestrel-config.yaml /etc/kestrel/config.yaml

EXPOSE 80
```

### Step 3: Create NGINX Config

```nginx
# nginx-plugin/nginx.conf
events {
    worker_connections 1024;
}

http {
    server {
        listen 80;
        root /var/www/html;
        index index.html;
    }
}
```

### Step 4: Create Static Content

```html
<!-- nginx-plugin/index.html -->
<!DOCTYPE html>
<html>
<body>
  <h1>Hello from Volant!</h1>
</body>
</html>
```

### Step 5: Configure Kestrel

```yaml
# nginx-plugin/kestrel-config.yaml
exec:
  - name: nginx
    command: /usr/sbin/nginx
    args: ["-g", "daemon off;"]
    
health:
  http:
    port: 80
    path: /
```

### Step 6: Create Plugin Manifest

```toml
# nginx-plugin/fledge.toml
[plugin]
name = "nginx-alpine"
version = "1.0.0"
description = "NGINX web server on Alpine Linux"

[plugin.runtime]
type = "oci"
source = "Dockerfile"

[plugin.manifest]
workload.type = "http"
workload.port = 80

resources.vcpu = 1
resources.memory_mb = 256
```

### Step 7: Build Plugin

```bash
# Build plugin (creates nginx-alpine.tar.gz)
fledge build

# Install plugin
volar plugins install ./nginx-alpine.tar.gz
```

### Step 8: Run VM

```bash
# Start VM from plugin
volar vms create my-nginx --plugin nginx-alpine

# Check status
volar vms list

# Access NGINX (bridge mode)
curl http://$(volar vms get my-nginx --output json | jq -r .ip)
```

---

## Advanced Dockerfile Patterns

### Multi-Stage Builds

Compile in one stage, run in minimal runtime:

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates && \
    wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel
    
COPY --from=builder /app /usr/local/bin/app
COPY kestrel-config.yaml /etc/kestrel/config.yaml

EXPOSE 8080
```

**Result**: Only the compiled binary and runtime dependencies make it into the final QCOW2 image.

### Dynamic Dependencies

Install packages at build time:

```dockerfile
FROM ubuntu:22.04

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y python3 python3-pip && \
    rm -rf /var/lib/apt/lists/*

# Install kestrel
RUN wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel

# Install Python packages
COPY requirements.txt /app/
RUN pip3 install -r /app/requirements.txt

COPY app.py /app/
COPY kestrel-config.yaml /etc/kestrel/config.yaml

WORKDIR /app
EXPOSE 5000
```

### Database Plugins

Persistent storage with initialization:

```dockerfile
FROM postgres:16-alpine

# Install kestrel
RUN wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel

# Initialize database on first boot
COPY init.sql /docker-entrypoint-initdb.d/
COPY kestrel-config.yaml /etc/kestrel/config.yaml

# Postgres data directory
VOLUME /var/lib/postgresql/data

EXPOSE 5432
```

**Note**: Kestrel config should start `postgres` directly, not use Docker's entrypoint script.

---

## Kestrel Agent Integration

All rootfs plugins must include the Kestrel agent to communicate with volantd.

### Installation in Dockerfile

```dockerfile
# Download latest kestrel binary
RUN wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel
```

### Configuration

Kestrel reads `/etc/kestrel/config.yaml`:

```yaml
# Process to manage
exec:
  - name: app
    command: /usr/local/bin/myapp
    args: ["--port", "8080"]
    env:
      - name: ENV
        value: production

# Health check
health:
  http:
    port: 8080
    path: /health
    interval: 10s
    timeout: 2s
    
# Lifecycle hooks (optional)
hooks:
  pre_start:
    - /scripts/migrate-db.sh
  post_stop:
    - /scripts/cleanup.sh
```

### Kestrel as PID 1

Fledge automatically configures the QCOW2 image to run kestrel as PID 1. You don't need an init system like systemd or runit.

```
Kernel → Kestrel (PID 1) → Your Application
```

Kestrel handles:
- Process supervision (restart on crash)
- Signal forwarding (SIGTERM for graceful shutdown)
- Health checks
- Lifecycle hooks

---

## Optimizing Image Size

Rootfs images can grow large. Apply these techniques:

### 1. Choose Minimal Base Images

```dockerfile
# Good: Alpine (5-7 MB base)
FROM alpine:3.19

# Better for static binaries: Distroless (2 MB base)
FROM gcr.io/distroless/static

# Avoid: Full Ubuntu (77 MB base)
FROM ubuntu:22.04
```

### 2. Clean Up in Same Layer

```dockerfile
# Bad: Each RUN creates a layer
RUN apt-get update
RUN apt-get install -y curl
RUN rm -rf /var/lib/apt/lists/*

# Good: Single layer, cleaned immediately
RUN apt-get update && \
    apt-get install -y curl && \
    rm -rf /var/lib/apt/lists/*
```

### 3. Use .dockerignore

```
# .dockerignore
.git
node_modules
*.log
test/
docs/
```

### 4. Multi-Stage Builds

Build artifacts in one stage, copy only binaries to final stage:

```dockerfile
FROM golang:1.21 AS builder
WORKDIR /build
COPY . .
RUN go build -o app

FROM alpine:3.19
COPY --from=builder /build/app /usr/local/bin/
```

### 5. Compress with QCOW2

Fledge automatically compresses the final QCOW2 image:

```toml
[plugin.runtime]
type = "oci"
source = "Dockerfile"
qcow2_compression = true  # Default: true
```

---

## Testing Rootfs Plugins

### Local Docker Testing

Test your Dockerfile before converting to QCOW2:

```bash
# Build image
docker build -t my-app:test .

# Run container
docker run -p 8080:8080 my-app:test

# Test endpoints
curl http://localhost:8080
```

**Caveat**: Kestrel won't run in Docker (it expects VM environment). Test your application logic, not kestrel integration.

### VM Testing

Once built as a plugin:

```bash
# Build plugin
fledge build

# Install plugin
volar plugins install ./my-app.tar.gz

# Create test VM
volar vms create test-vm --plugin my-app

# Check logs
volar vms logs test-vm

# Check health
volar vms get test-vm --output json | jq .health
```

### Debugging Failed Boots

If VM doesn't start:

```bash
# Check VM status
volar vms get my-vm

# View serial console output
volar vms logs my-vm --console

# SSH into VM (if cloud-init configured)
volar vms ssh my-vm

# Check kestrel logs
volar vms ssh my-vm 'journalctl -u kestrel'
```

---

## Common Patterns

### Environment Variable Injection

Pass config at runtime using cloud-init:

```yaml
# user-data.yaml
#cloud-config
write_files:
  - path: /etc/environment
    content: |
      DATABASE_URL=postgres://db.example.com/mydb
      API_KEY=secret123
```

```bash
volar vms create app-vm \
  --plugin my-app \
  --cloud-init user-data.yaml
```

Application reads from environment:

```python
# app.py
import os
db_url = os.getenv('DATABASE_URL')
```

### Volume Mounts (Future)

Persistent data volumes are on the roadmap. Current workaround: include storage in QCOW2 image.

### Secret Management

For sensitive data, use vsock to fetch secrets from host:

```yaml
# kestrel-config.yaml
exec:
  - name: fetch-secrets
    command: /scripts/fetch-secrets.sh
    vsock_cid: 2  # Host CID
    vsock_port: 9000

  - name: app
    command: /usr/local/bin/app
    depends_on: [fetch-secrets]
```

```bash
#!/bin/sh
# fetch-secrets.sh
socat VSOCK-CONNECT:2:9000 STDOUT > /secrets/api-key
```

---

## Real-World Examples

### WordPress Plugin

```dockerfile
FROM wordpress:6.4-php8.2-apache

# Install kestrel
RUN wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel

# Configure Apache and Kestrel
COPY apache-config.conf /etc/apache2/sites-available/000-default.conf
COPY kestrel-config.yaml /etc/kestrel/config.yaml

EXPOSE 80
```

```toml
# fledge.toml
[plugin]
name = "wordpress"
version = "6.4.0"

[plugin.manifest]
workload.type = "http"
workload.port = 80
resources.vcpu = 2
resources.memory_mb = 1024
```

### Node.js Application

```dockerfile
FROM node:20-alpine

RUN apk add --no-cache wget && \
    wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel

WORKDIR /app
COPY package*.json ./
RUN npm ci --production

COPY . .
COPY kestrel-config.yaml /etc/kestrel/config.yaml

EXPOSE 3000
```

```yaml
# kestrel-config.yaml
exec:
  - name: node-app
    command: /usr/local/bin/node
    args: ["server.js"]
    env:
      - name: NODE_ENV
        value: production
      - name: PORT
        value: "3000"

health:
  http:
    port: 3000
    path: /health
```

---

## Comparison: Rootfs vs Initramfs

| Feature | Rootfs (QCOW2) | Initramfs (RAM) |
|---------|----------------|-----------------|
| **Boot Speed** | ~2-3s | ~500ms-1s |
| **Artifact Size** | 100MB–2GB | 10–50MB |
| **Persistent Storage** | Yes (disk) | No (RAM only) |
| **Package Managers** | Yes | No |
| **Dynamic Dependencies** | Yes | No |
| **Memory Usage** | Lower (disk-backed) | Higher (all in RAM) |
| **Use Cases** | Full apps, databases | APIs, edge functions |
| **Complexity** | Medium (Dockerfile) | High (static linking) |

---

## Best Practices

1. **Always include kestrel** – Required for VM management and health checks
2. **Pin base image versions** – `alpine:3.19` not `alpine:latest` for reproducibility
3. **Use multi-stage builds** – Keep final image minimal
4. **Clean up in same layer** – Avoid layer bloat from temporary files
5. **Test Dockerfile first** – Validate with Docker before building QCOW2
6. **Minimize layers** – Combine RUN commands with `&&`
7. **Use .dockerignore** – Exclude unnecessary files from build context
8. **Document dependencies** – Comment why each package is needed
9. **Version your plugins** – Use semantic versioning in `fledge.toml`
10. **Include health checks** – Always configure kestrel health checks

---

## Next Steps

- **[Plugin Examples](4_examples.md)** – Real-world plugin architectures and patterns
- **[Manifest Schema Reference](../6_reference/1_manifest-schema.md)** – Complete manifest specification
- **[Architecture Overview](../5_architecture/1_overview.md)** – How rootfs plugins interact with volantd

---

## Troubleshooting

### Plugin Build Fails

```bash
# Check Docker daemon
docker info

# Validate Dockerfile syntax
docker build --no-cache -t test .

# Check fledge.toml syntax
fledge validate
```

### QCOW2 Image Too Large

```bash
# Check layer sizes
docker history my-app:latest

# Identify large files
docker run --rm my-app:latest du -sh /* | sort -h
```

### VM Fails to Boot

```bash
# Check serial console
volar vms logs my-vm --console

# Common issues:
# - Kestrel not installed
# - Kestrel config missing/invalid
# - Application crashes on startup
# - Missing dependencies
```

### Kestrel Not Starting

```bash
# Verify kestrel binary exists
volar vms ssh my-vm 'which kestrel'

# Check kestrel config
volar vms ssh my-vm 'cat /etc/kestrel/config.yaml'

# View kestrel logs
volar vms ssh my-vm 'journalctl -u kestrel -f'
```
