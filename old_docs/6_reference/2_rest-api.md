# REST API Reference

The Volant HTTP API provides programmatic access to VM management, plugin operations, deployments, and system monitoring. All endpoints are prefixed with `/api/v1`.

**Base URL**: `http://localhost:8080/api/v1` (default)

---

## Table of Contents

- [Authentication](#authentication)
- [Response Format](#response-format)
- [Error Handling](#error-handling)
- [Virtual Machines](#virtual-machines)
- [Deployments](#deployments)
- [Plugins](#plugins)
- [System](#system)
- [Events](#events)
- [OpenAPI Specification](#openapi-specification)

---

## Authentication

Currently, the Volant API does not require authentication when accessed locally. For production deployments, configure your reverse proxy or load balancer to handle authentication (OAuth2, mTLS, API keys).

```bash
# Access API directly
curl http://localhost:8080/api/v1/vms
```

---

## Response Format

### Success Responses

All successful responses return JSON with appropriate HTTP status codes:

```json
{
  "name": "my-vm",
  "status": "running",
  "plugin": "nginx-alpine",
  "ip": "10.0.0.5",
  "created_at": "2025-01-15T10:30:00Z"
}
```

### Pagination

List endpoints support pagination via query parameters:

```
GET /api/v1/vms?limit=50&offset=0
```

---

## Error Handling

### Error Response Format

```json
{
  "error": "VM not found",
  "code": "vm_not_found",
  "details": {
    "name": "nonexistent-vm"
  }
}
```

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 201 | Created |
| 204 | No Content (successful deletion) |
| 400 | Bad Request (invalid input) |
| 404 | Not Found |
| 409 | Conflict (resource already exists) |
| 500 | Internal Server Error |

---

## Virtual Machines

### List VMs

```http
GET /api/v1/vms
```

**Query Parameters**:
- `plugin` (optional): Filter by plugin name
- `status` (optional): Filter by status (`running`, `stopped`, `starting`)

**Response**: `200 OK`

```json
[
  {
    "name": "web-1",
    "status": "running",
    "plugin": "nginx-alpine",
    "ip": "10.0.0.10",
    "pid": 12345,
    "created_at": "2025-01-15T10:00:00Z",
    "resources": {
      "vcpu": 2,
      "memory_mb": 512
    }
  },
  {
    "name": "api-1",
    "status": "running",
    "plugin": "user-api",
    "ip": "10.0.0.11",
    "pid": 12346,
    "created_at": "2025-01-15T10:05:00Z"
  }
]
```

**Example**:

```bash
# List all VMs
curl http://localhost:8080/api/v1/vms

# Filter by plugin
curl http://localhost:8080/api/v1/vms?plugin=nginx-alpine

# Filter by status
curl http://localhost:8080/api/v1/vms?status=running
```

---

### Create VM

```http
POST /api/v1/vms
```

**Request Body**:

```json
{
  "name": "my-vm",
  "plugin": "nginx-alpine",
  "config": {
    "resources": {
      "vcpu": 2,
      "memory_mb": 512
    },
    "cloud_init": {
      "user_data": "#cloud-config\npackages:\n  - curl"
    }
  }
}
```

**Response**: `201 Created`

```json
{
  "name": "my-vm",
  "status": "starting",
  "plugin": "nginx-alpine",
  "ip": "10.0.0.12",
  "created_at": "2025-01-15T11:00:00Z"
}
```

**Example**:

```bash
curl -X POST http://localhost:8080/api/v1/vms \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-vm",
    "plugin": "nginx-alpine"
  }'
```

---

### Get VM

```http
GET /api/v1/vms/{name}
```

**Response**: `200 OK`

```json
{
  "name": "my-vm",
  "status": "running",
  "plugin": "nginx-alpine",
  "ip": "10.0.0.10",
  "pid": 12345,
  "created_at": "2025-01-15T10:00:00Z",
  "resources": {
    "vcpu": 2,
    "memory_mb": 512
  },
  "health": {
    "status": "healthy",
    "last_check": "2025-01-15T12:00:00Z"
  }
}
```

**Errors**:
- `404`: VM not found

**Example**:

```bash
curl http://localhost:8080/api/v1/vms/my-vm
```

---

### Start VM

```http
POST /api/v1/vms/{name}/start
```

Starts a stopped VM.

**Response**: `200 OK`

```json
{
  "name": "my-vm",
  "status": "starting",
  "plugin": "nginx-alpine"
}
```

**Errors**:
- `404`: VM not found
- `409`: VM already running

**Example**:

```bash
curl -X POST http://localhost:8080/api/v1/vms/my-vm/start
```

---

### Stop VM

```http
POST /api/v1/vms/{name}/stop
```

Gracefully stops a running VM (sends SIGTERM, waits for clean shutdown).

**Query Parameters**:
- `force` (optional): Force stop without graceful shutdown (default: `false`)

**Response**: `200 OK`

```json
{
  "name": "my-vm",
  "status": "stopped",
  "plugin": "nginx-alpine"
}
```

**Errors**:
- `404`: VM not found
- `409`: VM already stopped

**Example**:

```bash
# Graceful stop
curl -X POST http://localhost:8080/api/v1/vms/my-vm/stop

# Force stop
curl -X POST "http://localhost:8080/api/v1/vms/my-vm/stop?force=true"
```

---

### Restart VM

```http
POST /api/v1/vms/{name}/restart
```

Restarts a VM (stop + start).

**Response**: `200 OK`

```json
{
  "name": "my-vm",
  "status": "restarting",
  "plugin": "nginx-alpine"
}
```

**Errors**:
- `404`: VM not found

**Example**:

```bash
curl -X POST http://localhost:8080/api/v1/vms/my-vm/restart
```

---

### Delete VM

```http
DELETE /api/v1/vms/{name}
```

Stops and deletes a VM.

**Query Parameters**:
- `force` (optional): Force delete without graceful shutdown (default: `false`)

**Response**: `204 No Content`

**Errors**:
- `404`: VM not found

**Example**:

```bash
curl -X DELETE http://localhost:8080/api/v1/vms/my-vm
```

---

### Get VM Configuration

```http
GET /api/v1/vms/{name}/config
```

Retrieves the current configuration of a VM.

**Response**: `200 OK`

```json
{
  "resources": {
    "vcpu": 2,
    "memory_mb": 512
  },
  "network": {
    "mode": "bridge",
    "bridge_name": "volbr0"
  },
  "cloud_init": {
    "enabled": true
  }
}
```

**Example**:

```bash
curl http://localhost:8080/api/v1/vms/my-vm/config
```

---

### Update VM Configuration

```http
PATCH /api/v1/vms/{name}/config
```

Updates VM configuration (requires VM restart to take effect).

**Request Body**:

```json
{
  "resources": {
    "vcpu": 4,
    "memory_mb": 1024
  }
}
```

**Response**: `200 OK`

```json
{
  "resources": {
    "vcpu": 4,
    "memory_mb": 1024
  },
  "restart_required": true
}
```

**Example**:

```bash
curl -X PATCH http://localhost:8080/api/v1/vms/my-vm/config \
  -H "Content-Type: application/json" \
  -d '{
    "resources": {
      "vcpu": 4
    }
  }'
```

---

### Get VM Configuration History

```http
GET /api/v1/vms/{name}/config/history
```

Retrieves the configuration change history for a VM.

**Response**: `200 OK`

```json
[
  {
    "timestamp": "2025-01-15T12:00:00Z",
    "changes": {
      "resources.vcpu": {
        "old": 2,
        "new": 4
      }
    },
    "author": "api"
  },
  {
    "timestamp": "2025-01-15T10:00:00Z",
    "changes": {
      "created": true
    },
    "author": "api"
  }
]
```

**Example**:

```bash
curl http://localhost:8080/api/v1/vms/my-vm/config/history
```

---

### Get VM Plugin OpenAPI Spec

```http
GET /api/v1/vms/{name}/openapi
```

Retrieves the OpenAPI specification for the VM's plugin workload (if the plugin exposes an HTTP API).

**Response**: `200 OK`

```json
{
  "openapi": "3.0.3",
  "info": {
    "title": "nginx-alpine API",
    "version": "1.0.0"
  },
  "paths": {
    "/": {
      "get": {
        "summary": "Serve static content"
      }
    }
  }
}
```

**Errors**:
- `404`: VM not found or plugin has no OpenAPI spec

**Example**:

```bash
curl http://localhost:8080/api/v1/vms/my-vm/openapi
```

---

## Deployments

### List Deployments

```http
GET /api/v1/deployments
```

**Response**: `200 OK`

```json
[
  {
    "name": "api",
    "plugin": "user-api",
    "replicas": 3,
    "ready_replicas": 3,
    "vms": ["api-1", "api-2", "api-3"],
    "created_at": "2025-01-15T10:00:00Z"
  }
]
```

**Example**:

```bash
curl http://localhost:8080/api/v1/deployments
```

---

### Create Deployment

```http
POST /api/v1/deployments
```

**Request Body**:

```json
{
  "name": "api",
  "plugin": "user-api",
  "replicas": 3
}
```

**Response**: `201 Created`

```json
{
  "name": "api",
  "plugin": "user-api",
  "replicas": 3,
  "ready_replicas": 0,
  "vms": [],
  "created_at": "2025-01-15T11:00:00Z"
}
```

**Errors**:
- `400`: Invalid configuration
- `404`: Plugin not found
- `409`: Deployment already exists

**Example**:

```bash
curl -X POST http://localhost:8080/api/v1/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web",
    "plugin": "nginx-alpine",
    "replicas": 5
  }'
```

---

### Get Deployment

```http
GET /api/v1/deployments/{name}
```

**Response**: `200 OK`

```json
{
  "name": "api",
  "plugin": "user-api",
  "replicas": 3,
  "ready_replicas": 3,
  "vms": ["api-1", "api-2", "api-3"],
  "created_at": "2025-01-15T10:00:00Z",
  "status": {
    "phase": "running",
    "conditions": [
      {
        "type": "available",
        "status": "true",
        "last_update": "2025-01-15T10:05:00Z"
      }
    ]
  }
}
```

**Errors**:
- `404`: Deployment not found

**Example**:

```bash
curl http://localhost:8080/api/v1/deployments/api
```

---

### Scale Deployment

```http
PATCH /api/v1/deployments/{name}
```

**Request Body**:

```json
{
  "replicas": 5
}
```

**Response**: `200 OK`

```json
{
  "name": "api",
  "plugin": "user-api",
  "replicas": 5,
  "ready_replicas": 3,
  "vms": ["api-1", "api-2", "api-3"],
  "scaling": true
}
```

**Errors**:
- `400`: Invalid replica count
- `404`: Deployment not found

**Example**:

```bash
curl -X PATCH http://localhost:8080/api/v1/deployments/api \
  -H "Content-Type: application/json" \
  -d '{"replicas": 10}'
```

---

### Delete Deployment

```http
DELETE /api/v1/deployments/{name}
```

Deletes a deployment and all its VMs.

**Response**: `204 No Content`

**Errors**:
- `404`: Deployment not found

**Example**:

```bash
curl -X DELETE http://localhost:8080/api/v1/deployments/api
```

---

## Plugins

### List Plugins

```http
GET /api/v1/plugins
```

**Response**: `200 OK`

```json
{
  "plugins": [
    "nginx-alpine",
    "user-api",
    "postgres"
  ]
}
```

**Example**:

```bash
curl http://localhost:8080/api/v1/plugins
```

---

### Install Plugin

```http
POST /api/v1/plugins
```

Installs a plugin from a manifest.

**Request Body**:

```json
{
  "name": "my-app",
  "version": "1.0.0",
  "description": "My custom application",
  "workload": {
    "type": "http",
    "port": 8080
  },
  "resources": {
    "vcpu": 2,
    "memory_mb": 512
  },
  "rootfs": {
    "format": "qcow2",
    "url": "https://plugins.example.com/my-app-1.0.0.qcow2",
    "checksum": "sha256:abc123..."
  }
}
```

**Response**: `201 Created`

**Errors**:
- `400`: Invalid manifest
- `409`: Plugin already installed

**Example**:

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d @manifest.json
```

---

### Get Plugin

```http
GET /api/v1/plugins/{plugin}
```

**Response**: `200 OK`

```json
{
  "name": "nginx-alpine",
  "version": "1.0.0",
  "description": "NGINX web server on Alpine Linux",
  "workload": {
    "type": "http",
    "port": 80
  },
  "resources": {
    "vcpu": 1,
    "memory_mb": 256
  },
  "installed_at": "2025-01-15T09:00:00Z"
}
```

**Errors**:
- `404`: Plugin not found

**Example**:

```bash
curl http://localhost:8080/api/v1/plugins/nginx-alpine
```

---

### Remove Plugin

```http
DELETE /api/v1/plugins/{plugin}
```

Removes a plugin (fails if any VMs are using it).

**Response**: `204 No Content`

**Errors**:
- `404`: Plugin not found
- `409`: Plugin in use by running VMs

**Example**:

```bash
curl -X DELETE http://localhost:8080/api/v1/plugins/nginx-alpine
```

---

### Set Plugin Enabled Status

```http
POST /api/v1/plugins/{plugin}/enabled
```

Enables or disables a plugin (disabled plugins cannot be used for new VMs).

**Request Body**:

```json
{
  "enabled": false
}
```

**Response**: `200 OK`

**Errors**:
- `404`: Plugin not found

**Example**:

```bash
# Disable plugin
curl -X POST http://localhost:8080/api/v1/plugins/nginx-alpine/enabled \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'

# Enable plugin
curl -X POST http://localhost:8080/api/v1/plugins/nginx-alpine/enabled \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

---

## System

### Get System Information

```http
GET /api/v1/system/info
```

**Response**: `200 OK`

```json
{
  "version": "0.1.0",
  "commit": "abc123",
  "build_date": "2025-01-15T00:00:00Z",
  "go_version": "go1.21.5",
  "platform": "linux/amd64",
  "stats": {
    "total_vms": 12,
    "running_vms": 10,
    "total_deployments": 3,
    "total_plugins": 5
  },
  "resources": {
    "cpu_count": 8,
    "memory_total_mb": 16384,
    "memory_available_mb": 8192
  }
}
```

**Example**:

```bash
curl http://localhost:8080/api/v1/system/info
```

---

## Events

### Stream VM Lifecycle Events

```http
GET /api/v1/events/vms
```

Streams VM lifecycle events via Server-Sent Events (SSE).

**Response**: `200 OK` (streaming)

**Content-Type**: `text/event-stream`

**Event Format**:

```
event: vm.created
data: {"name":"my-vm","plugin":"nginx-alpine","timestamp":"2025-01-15T10:00:00Z"}

event: vm.started
data: {"name":"my-vm","ip":"10.0.0.10","timestamp":"2025-01-15T10:00:05Z"}

event: vm.stopped
data: {"name":"my-vm","timestamp":"2025-01-15T10:30:00Z"}

event: vm.deleted
data: {"name":"my-vm","timestamp":"2025-01-15T10:31:00Z"}
```

**Event Types**:
- `vm.created` – VM created
- `vm.started` – VM started successfully
- `vm.stopped` – VM stopped
- `vm.deleted` – VM deleted
- `vm.health_changed` – Health status changed
- `vm.error` – VM encountered an error

**Example**:

```bash
# Stream events with curl
curl http://localhost:8080/api/v1/events/vms

# Process events with jq
curl -N http://localhost:8080/api/v1/events/vms | \
  while read line; do
    if [[ $line == data:* ]]; then
      echo "$line" | sed 's/^data: //' | jq .
    fi
  done
```

**Example (JavaScript)**:

```javascript
const eventSource = new EventSource('http://localhost:8080/api/v1/events/vms');

eventSource.addEventListener('vm.started', (event) => {
  const data = JSON.parse(event.data);
  console.log('VM started:', data.name, data.ip);
});

eventSource.addEventListener('vm.stopped', (event) => {
  const data = JSON.parse(event.data);
  console.log('VM stopped:', data.name);
});
```

---

## OpenAPI Specification

### Get OpenAPI Spec

```http
GET /api/v1/openapi
```

Returns the complete OpenAPI 3.0 specification for the Volant API.

**Response**: `200 OK`

```json
{
  "openapi": "3.0.3",
  "info": {
    "title": "Volant API",
    "version": "v1",
    "description": "VM orchestration and plugin management"
  },
  "paths": {
    "/api/v1/vms": { ... },
    "/api/v1/deployments": { ... }
  }
}
```

**Example**:

```bash
# Download spec
curl http://localhost:8080/api/v1/openapi > volant-openapi.json

# Generate client SDK
openapi-generator-cli generate \
  -i volant-openapi.json \
  -g python \
  -o ./volant-client-python
```

---

## Rate Limiting

The API does not currently implement rate limiting. For production deployments, configure rate limiting at your reverse proxy or load balancer.

---

## Webhooks

Webhooks are not currently supported. Use the [SSE event stream](#stream-vm-lifecycle-events) for real-time notifications.

---

## SDK Examples

### Python

```python
import requests

# List VMs
response = requests.get('http://localhost:8080/api/v1/vms')
vms = response.json()

for vm in vms:
    print(f"{vm['name']}: {vm['status']} ({vm['ip']})")

# Create VM
response = requests.post('http://localhost:8080/api/v1/vms', json={
    'name': 'test-vm',
    'plugin': 'nginx-alpine'
})
vm = response.json()
print(f"Created: {vm['name']}")
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type VM struct {
    Name   string `json:"name"`
    Plugin string `json:"plugin"`
    Status string `json:"status"`
    IP     string `json:"ip"`
}

func main() {
    // List VMs
    resp, _ := http.Get("http://localhost:8080/api/v1/vms")
    var vms []VM
    json.NewDecoder(resp.Body).Decode(&vms)
    
    for _, vm := range vms {
        fmt.Printf("%s: %s (%s)\n", vm.Name, vm.Status, vm.IP)
    }
    
    // Create VM
    createReq := map[string]string{
        "name":   "test-vm",
        "plugin": "nginx-alpine",
    }
    body, _ := json.Marshal(createReq)
    resp, _ = http.Post("http://localhost:8080/api/v1/vms",
        "application/json", bytes.NewReader(body))
    
    var vm VM
    json.NewDecoder(resp.Body).Decode(&vm)
    fmt.Printf("Created: %s\n", vm.Name)
}
```

### JavaScript/Node.js

```javascript
const axios = require('axios');

const api = axios.create({
  baseURL: 'http://localhost:8080/api/v1'
});

// List VMs
async function listVMs() {
  const response = await api.get('/vms');
  response.data.forEach(vm => {
    console.log(`${vm.name}: ${vm.status} (${vm.ip})`);
  });
}

// Create VM
async function createVM(name, plugin) {
  const response = await api.post('/vms', {
    name,
    plugin
  });
  console.log('Created:', response.data.name);
}

listVMs();
createVM('test-vm', 'nginx-alpine');
```

---

## Next Steps

- **[CLI Reference](3_cli-reference.md)** – Command-line interface documentation
- **[Manifest Schema](1_manifest-schema.md)** – Plugin manifest specification
- **[Architecture Overview](../5_architecture/1_overview.md)** – System architecture details
