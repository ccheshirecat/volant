# Plugin Examples

This guide showcases **real-world plugin architectures** and patterns, demonstrating how to build production-grade plugins for common use cases. Each example includes complete code, manifest configuration, and deployment strategies.

---

## Example 1: Static Website (Initramfs)

A minimal static file server built with Caddy, deployed as an initramfs plugin for ultra-fast boot times.

### Use Case
- Landing pages, documentation sites, marketing content
- Sub-second boot time required
- No dynamic content or server-side processing

### Architecture

```
Initramfs (RAM)
├── caddy (static binary)
├── index.html
├── assets/
│   ├── style.css
│   └── app.js
└── kestrel agent
```

### Files

```toml
# fledge.toml
[plugin]
name = "static-site"
version = "1.0.0"
description = "Static website with Caddy"

[plugin.runtime]
type = "initramfs"
build_script = "./build.sh"

[plugin.manifest]
workload.type = "http"
workload.port = 8080

resources.vcpu = 1
resources.memory_mb = 128
```

```bash
#!/bin/bash
# build.sh

set -e

# Download statically-linked Caddy
wget -O caddy https://github.com/caddyserver/caddy/releases/download/v2.7.6/caddy_2.7.6_linux_amd64
chmod +x caddy

# Download Kestrel agent
wget -O kestrel https://get.volantvm.com/kestrel/latest/linux-amd64
chmod +x kestrel

# Create directory structure
mkdir -p rootfs/{bin,etc/kestrel,var/www}

# Install binaries
cp caddy rootfs/bin/
cp kestrel rootfs/bin/

# Copy website content
cp -r site/* rootfs/var/www/

# Create Kestrel config
cat > rootfs/etc/kestrel/config.yaml <<'EOF'
exec:
  - name: caddy
    command: /bin/caddy
    args: ["file-server", "--root", "/var/www", "--listen", ":8080"]
    
health:
  http:
    port: 8080
    path: /
EOF

# Create initramfs
cd rootfs
find . | cpio -o -H newc | gzip > ../initramfs.cpio.gz
```

```html
<!-- site/index.html -->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Volant Static Site</title>
  <link rel="stylesheet" href="/assets/style.css">
</head>
<body>
  <h1>Hello from Volant!</h1>
  <p>This site boots in under 500ms.</p>
  <script src="/assets/app.js"></script>
</body>
</html>
```

### Build & Deploy

```bash
# Build plugin
fledge build

# Install plugin
volar plugins install ./static-site.tar.gz

# Create VM
volar vms create website --plugin static-site

# Access site
curl http://$(volar vms get website -o json | jq -r .ip):8080
```

### Performance Characteristics
- **Build time**: ~10 seconds
- **Artifact size**: ~15MB
- **Boot time**: ~500ms
- **Memory usage**: 64MB actual (128MB allocated)

---

## Example 2: REST API (Initramfs + Go)

A statically-compiled Go API with PostgreSQL connection, packaged as an initramfs plugin.

### Use Case
- Microservices, backend APIs
- Fast startup for autoscaling
- Stateless workload (database on separate VM)

### Architecture

```
Initramfs (RAM)
├── api (static Go binary)
├── migrations/ (SQL files)
├── config.yaml
└── kestrel agent
```

### Files

```toml
# fledge.toml
[plugin]
name = "user-api"
version = "2.1.0"
description = "User management REST API"

[plugin.runtime]
type = "initramfs"
build_script = "./build.sh"

[plugin.manifest]
workload.type = "http"
workload.port = 8080
workload.health_path = "/health"

resources.vcpu = 2
resources.memory_mb = 512

[plugin.manifest.env]
DATABASE_URL = "postgres://api:pass@db.internal/users"
LOG_LEVEL = "info"
```

```go
// main.go
package main

import (
    "database/sql"
    "encoding/json"
    "log"
    "net/http"
    "os"
    
    _ "github.com/lib/pq"
)

type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

var db *sql.DB

func main() {
    var err error
    db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    http.HandleFunc("/health", healthHandler)
    http.HandleFunc("/users", usersHandler)
    
    log.Println("Listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    if err := db.Ping(); err != nil {
        http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
        return
    }
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query("SELECT id, name, email FROM users")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()
    
    var users []User
    for rows.Next() {
        var u User
        rows.Scan(&u.ID, &u.Name, &u.Email)
        users = append(users, u)
    }
    
    json.NewEncoder(w).Encode(users)
}
```

```bash
#!/bin/bash
# build.sh

set -e

# Build static Go binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o api .

# Download Kestrel
wget -O kestrel https://get.volantvm.com/kestrel/latest/linux-amd64
chmod +x kestrel

# Create initramfs structure
mkdir -p rootfs/{bin,etc/kestrel,migrations}

cp api rootfs/bin/
cp kestrel rootfs/bin/
cp migrations/*.sql rootfs/migrations/

# Kestrel config
cat > rootfs/etc/kestrel/config.yaml <<'EOF'
exec:
  - name: migrate
    command: /bin/api
    args: ["migrate"]
    run_once: true
    
  - name: api
    command: /bin/api
    depends_on: [migrate]
    env:
      - name: DATABASE_URL
        value_from: env  # Read from VM environment
      - name: LOG_LEVEL
        value_from: env

health:
  http:
    port: 8080
    path: /health
    interval: 5s
    timeout: 2s
EOF

# Package initramfs
cd rootfs
find . | cpio -o -H newc | gzip > ../initramfs.cpio.gz
```

### Deployment Pattern

```bash
# Build plugin
fledge build

# Install plugin
volar plugins install ./user-api.tar.gz

# Create deployment with 3 replicas
cat > deployment.json <<EOF
{
  "name": "user-api",
  "plugin": "user-api",
  "replicas": 3
}
EOF

volar deployments create -f deployment.json

# Scale up
volar deployments scale user-api --replicas 10
```

### Performance Characteristics
- **Build time**: ~30 seconds (Go compilation)
- **Artifact size**: ~20MB
- **Boot time**: ~800ms
- **Cold start to first request**: <1 second

---

## Example 3: Python Web App (Rootfs)

A Flask application with dependencies, packaged as a rootfs plugin with Dockerfile.

### Use Case
- Web applications with Python dependencies
- Dynamic content rendering
- File uploads, session management

### Architecture

```
QCOW2 Disk Image
├── Python 3.11
├── Flask + dependencies (pip)
├── Application code
├── Static assets
├── SQLite database
└── Kestrel agent
```

### Files

```dockerfile
# Dockerfile
FROM python:3.11-slim

# Install Kestrel
RUN apt-get update && \
    apt-get install -y wget && \
    wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel && \
    apt-get remove -y wget && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY app.py .
COPY templates/ templates/
COPY static/ static/

# Kestrel config
COPY kestrel-config.yaml /etc/kestrel/config.yaml

# Initialize database on first boot
COPY init_db.py .
RUN python init_db.py

EXPOSE 5000
```

```python
# app.py
from flask import Flask, render_template, request, jsonify
import sqlite3

app = Flask(__name__)

def get_db():
    conn = sqlite3.connect('/app/data/app.db')
    conn.row_factory = sqlite3.Row
    return conn

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/api/items', methods=['GET', 'POST'])
def items():
    db = get_db()
    
    if request.method == 'POST':
        data = request.get_json()
        db.execute('INSERT INTO items (name, description) VALUES (?, ?)',
                   (data['name'], data['description']))
        db.commit()
        return jsonify({'status': 'created'}), 201
    
    items = db.execute('SELECT * FROM items').fetchall()
    return jsonify([dict(row) for row in items])

@app.route('/health')
def health():
    return jsonify({'status': 'healthy'})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
```

```yaml
# kestrel-config.yaml
exec:
  - name: flask
    command: /usr/local/bin/python
    args: ["app.py"]
    working_dir: /app
    env:
      - name: FLASK_ENV
        value: production

health:
  http:
    port: 5000
    path: /health
    interval: 10s
```

```toml
# fledge.toml
[plugin]
name = "flask-app"
version = "1.0.0"
description = "Flask web application with SQLite"

[plugin.runtime]
type = "oci"
source = "Dockerfile"

[plugin.manifest]
workload.type = "http"
workload.port = 5000

resources.vcpu = 2
resources.memory_mb = 1024
```

### Build & Deploy

```bash
# Build plugin (takes 2-3 minutes)
fledge build

# Install plugin
volar plugins install ./flask-app.tar.gz

# Create VM with cloud-init for secrets
cat > user-data.yaml <<EOF
#cloud-config
write_files:
  - path: /app/.env
    content: |
      SECRET_KEY=prod-secret-key-here
      DATABASE_URL=/app/data/app.db
EOF

volar vms create app \
  --plugin flask-app \
  --cloud-init user-data.yaml
```

### Performance Characteristics
- **Build time**: 2-3 minutes (Docker build + QCOW2 conversion)
- **Artifact size**: ~400MB
- **Boot time**: ~2-3 seconds
- **Memory usage**: 512MB actual (1GB allocated)

---

## Example 4: Database Server (Rootfs)

PostgreSQL database packaged as a plugin with persistent storage.

### Use Case
- Managed database instances
- Development databases
- Isolated per-tenant databases

### Architecture

```
QCOW2 Disk Image
├── PostgreSQL 16
├── Database files (/var/lib/postgresql/data)
├── Initialization scripts
└── Kestrel agent
```

### Files

```dockerfile
# Dockerfile
FROM postgres:16-alpine

# Install Kestrel
RUN apk add --no-cache wget && \
    wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel && \
    apk del wget

# Copy initialization scripts
COPY init-scripts/*.sql /docker-entrypoint-initdb.d/

# Kestrel configuration
COPY kestrel-config.yaml /etc/kestrel/config.yaml

# PostgreSQL data directory
VOLUME /var/lib/postgresql/data

EXPOSE 5432
```

```yaml
# kestrel-config.yaml
exec:
  - name: postgres
    command: /usr/local/bin/postgres
    args: ["-D", "/var/lib/postgresql/data"]
    user: postgres
    env:
      - name: POSTGRES_PASSWORD
        value_from: env
      - name: POSTGRES_DB
        value: myapp

health:
  tcp:
    port: 5432
    interval: 10s
    
hooks:
  pre_start:
    - /scripts/init-db.sh
```

```sql
-- init-scripts/01-schema.sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
```

```toml
# fledge.toml
[plugin]
name = "postgres"
version = "16.1.0"
description = "PostgreSQL 16 database server"

[plugin.runtime]
type = "oci"
source = "Dockerfile"

[plugin.manifest]
workload.type = "tcp"
workload.port = 5432

resources.vcpu = 2
resources.memory_mb = 2048

[plugin.manifest.env]
POSTGRES_PASSWORD = "changeme"
```

### Deployment with Persistent Storage

```bash
# Build plugin
fledge build

# Install plugin
volar plugins install ./postgres.tar.gz

# Create VM with custom password via cloud-init
cat > db-config.yaml <<EOF
#cloud-config
write_files:
  - path: /etc/environment
    content: |
      POSTGRES_PASSWORD=secure-production-password
      POSTGRES_DB=production
EOF

volar vms create prod-db \
  --plugin postgres \
  --cloud-init db-config.yaml

# Connect from host
export DB_IP=$(volar vms get prod-db -o json | jq -r .ip)
psql -h $DB_IP -U postgres -d production
```

---

## Example 5: Multi-Tier Application

Complete 3-tier architecture: NGINX reverse proxy → API → PostgreSQL

### Architecture

```
NGINX VM (static-proxy plugin)
    ↓
API VM Deployment (3 replicas, user-api plugin)
    ↓
PostgreSQL VM (postgres plugin)
```

### Deployment Strategy

```bash
# 1. Deploy database
volar vms create app-db --plugin postgres

# 2. Deploy API tier (3 replicas)
cat > api-deployment.json <<EOF
{
  "name": "api",
  "plugin": "user-api",
  "replicas": 3
}
EOF

volar deployments create -f api-deployment.json

# 3. Deploy NGINX proxy
cat > nginx.conf <<'EOF'
upstream api_backend {
    server 10.0.0.10:8080;  # API replica 1
    server 10.0.0.11:8080;  # API replica 2
    server 10.0.0.12:8080;  # API replica 3
}

server {
    listen 80;
    
    location /api/ {
        proxy_pass http://api_backend;
    }
    
    location / {
        root /var/www;
    }
}
EOF

# Create custom NGINX plugin with this config
# (build process omitted for brevity)

volar vms create web --plugin nginx-proxy
```

### Service Discovery Pattern

For dynamic service discovery, use volantd's event stream:

```bash
# Stream VM events to update proxy config
volar events vms --watch | jq -r \
  'select(.event=="vm.started" and .plugin=="user-api") | .ip' \
  | while read ip; do
      # Update NGINX upstream config
      echo "server $ip:8080;" >> /etc/nginx/upstreams.conf
      nginx -s reload
    done
```

---

## Example 6: Scheduled Jobs (Cron)

Background job processor with cron scheduling.

### Use Case
- Data processing pipelines
- Scheduled reports
- Batch ETL jobs

### Files

```dockerfile
# Dockerfile
FROM alpine:3.19

RUN apk add --no-cache python3 py3-pip dcron wget && \
    wget -O /usr/local/bin/kestrel https://get.volantvm.com/kestrel/latest/linux-amd64 && \
    chmod +x /usr/local/bin/kestrel

COPY requirements.txt /app/
RUN pip3 install -r /app/requirements.txt

COPY jobs/ /app/jobs/
COPY kestrel-config.yaml /etc/kestrel/config.yaml

# Crontab
COPY crontab /etc/crontabs/root
```

```
# crontab
# Run data sync every hour
0 * * * * /usr/bin/python3 /app/jobs/sync_data.py >> /var/log/sync.log 2>&1

# Generate daily report at 2 AM
0 2 * * * /usr/bin/python3 /app/jobs/daily_report.py >> /var/log/report.log 2>&1

# Cleanup old files every Sunday
0 0 * * 0 /app/jobs/cleanup.sh >> /var/log/cleanup.log 2>&1
```

```yaml
# kestrel-config.yaml
exec:
  - name: crond
    command: /usr/sbin/crond
    args: ["-f", "-l", "2"]

health:
  exec:
    command: /bin/ps
    args: ["aux"]
    success_pattern: "crond"
    interval: 30s
```

---

## Pattern: Blue-Green Deployments

Deploy new version alongside old, switch traffic atomically.

```bash
# Deploy v1 (blue)
volar deployments create \
  --name api-blue \
  --plugin user-api:1.0.0 \
  --replicas 5

# Deploy v2 (green)
volar deployments create \
  --name api-green \
  --plugin user-api:2.0.0 \
  --replicas 5

# Test green deployment
curl http://green-api-lb.internal/health

# Switch traffic (update load balancer config)
# ... external LB reconfiguration ...

# Decommission blue
volar deployments delete api-blue
```

---

## Pattern: Canary Releases

Gradually shift traffic to new version.

```bash
# Deploy v1 (90% traffic)
volar deployments create \
  --name api-v1 \
  --plugin user-api:1.0.0 \
  --replicas 9

# Deploy v2 (10% traffic)
volar deployments create \
  --name api-v2 \
  --plugin user-api:2.0.0 \
  --replicas 1

# Monitor metrics
# If successful, gradually increase v2 replicas

volar deployments scale api-v2 --replicas 5  # 50/50 split
volar deployments scale api-v1 --replicas 5

# Eventually
volar deployments scale api-v2 --replicas 10
volar deployments delete api-v1
```

---

## Best Practices Summary

### 1. Choose the Right Plugin Type

| Workload | Plugin Type | Reason |
|----------|-------------|--------|
| Static sites | Initramfs | Fast boot, minimal resources |
| Stateless APIs | Initramfs | Quick scaling, low memory |
| Web apps with deps | Rootfs | Package managers, persistence |
| Databases | Rootfs | Persistent storage required |
| Batch jobs | Rootfs | Scheduling, cron support |

### 2. Health Checks Are Critical

Always configure appropriate health checks:

```yaml
# HTTP services
health:
  http:
    port: 8080
    path: /health

# TCP services (databases)
health:
  tcp:
    port: 5432

# Custom checks
health:
  exec:
    command: /scripts/check-health.sh
```

### 3. Use Environment Variables for Config

Never hard-code secrets or environment-specific config:

```toml
[plugin.manifest.env]
DATABASE_URL = "postgres://localhost/dev"  # Default for dev
LOG_LEVEL = "info"
```

Override at runtime with cloud-init:

```yaml
#cloud-config
write_files:
  - path: /etc/environment
    content: |
      DATABASE_URL=postgres://prod-db.internal/myapp
      LOG_LEVEL=warn
```

### 4. Version Your Plugins

Use semantic versioning:

```toml
[plugin]
name = "my-api"
version = "2.1.3"  # major.minor.patch
```

Pin dependencies:

```dockerfile
FROM python:3.11.6-slim  # Not :latest
RUN pip install flask==3.0.0  # Not >=3.0.0
```

### 5. Optimize for Your Use Case

**Initramfs** (speed-critical):
- Strip binaries: `go build -ldflags="-s -w"`
- Compress aggressively: `gzip -9`
- Minimize dependencies

**Rootfs** (size-critical):
- Use Alpine base images
- Multi-stage builds
- Clean package caches

---

## Next Steps

- **[Manifest Schema Reference](../6_reference/1_manifest-schema.md)** – Complete manifest specification
- **[REST API Reference](../6_reference/2_rest-api.md)** – HTTP API endpoints
- **[Scaling Guide](../3_guides/3_scaling.md)** – Production deployment strategies

---

## Further Reading

- **Kestrel Agent Documentation** – Process supervision and health checks
- **Cloud-Init Guide** – Dynamic VM configuration
- **Networking Guide** – Bridge vs vsock architecture
