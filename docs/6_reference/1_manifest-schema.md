# Plugin Manifest Schema Reference

Complete specification for the Volant plugin manifest format.

---

## Overview

The **plugin manifest** is a JSON file that describes how Volant should run your application. It contains metadata, boot configuration, resource requirements, workload details, and health check specifications.

**Schema Version**: `1.0`

---

## Complete Example

```json
{
  "$schema": "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json",
  "schema_version": "1.0",
  "name": "nginx",
  "version": "1.0.0",
  "runtime": "nginx",
  "enabled": true,
  
  "initramfs": {
    "url": "/var/lib/volant/plugins/nginx/plugin.cpio.gz",
    "checksum": "sha256:abc123def456..."
  },
  
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 1024
  },
  
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/sbin/nginx", "-g", "daemon off;"],
    "base_url": "http://127.0.0.1:80",
    "workdir": "/",
    "env": {
      "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
      "NGINX_VERSION": "1.25.0"
    }
  },
  
  "health_check": {
    "endpoint": "/health",
    "timeout_ms": 10000,
    "interval_ms": 5000,
    "retries": 3
  },
  
  "network": {
    "mode": "bridged"
  },
  
  "cloud_init": {
    "datasource": "NoCloud",
    "seed_mode": "disk"
  }
}
```

---

## Top-Level Fields

### `$schema` (optional)

- **Type**: `string` (URL)
- **Description**: JSON Schema URL for validation and IDE support
- **Example**: `"https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json"`

### `schema_version` (required)

- **Type**: `string`
- **Description**: Manifest schema version
- **Value**: `"1.0"` (currently the only supported version)

### `name` (required)

- **Type**: `string`
- **Description**: Unique plugin identifier
- **Rules**:
  - Must be lowercase
  - Alphanumeric characters, hyphens, and underscores only
  - No spaces
- **Examples**: `"nginx"`, `"my-app"`, `"postgres_15"`

### `version` (required)

- **Type**: `string`
- **Description**: Plugin version (semantic versioning recommended)
- **Examples**: `"1.0.0"`, `"2.3.1-alpha"`, `"v0.5.0"`

### `runtime` (required)

- **Type**: `string`
- **Description**: Runtime identifier (typically same as `name`)
- **Purpose**: Allows multiple plugins to share the same runtime
- **Example**: `"nginx"`

### `enabled` (required)

- **Type**: `boolean`
- **Description**: Whether the plugin can be used to create VMs
- **Values**: `true` or `false`
- **Default**: `true`

---

## Boot Media

Exactly **one** of `initramfs` or `rootfs` must be specified.

### `initramfs` (conditional)

Specify this for initramfs-based plugins.

```json
{
  "initramfs": {
    "url": "/var/lib/volant/plugins/myapp/plugin.cpio.gz",
    "checksum": "sha256:abc123..."
  }
}
```

**Fields**:

- **`url`** (required): Path to `.cpio.gz` file (absolute path recommended)
- **`checksum`** (required): SHA256 checksum prefixed with `sha256:`

### `rootfs` (conditional)

Specify this for rootfs-based plugins.

```json
{
  "rootfs": {
    "url": "/var/lib/volant/plugins/nginx/rootfs.img",
    "checksum": "sha256:def456...",
    "format": "ext4"
  }
}
```

**Fields**:

- **`url`** (required): Path to `.img` file (absolute path recommended)
- **`checksum`** (required): SHA256 checksum prefixed with `sha256:`
- **`format`** (required): Filesystem type (`"ext4"`, `"xfs"`, or `"btrfs"`)

---

## Resources

Default resource allocations for VMs created with this plugin.

```json
{
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 1024
  }
}
```

### `resources.cpu_cores` (required)

- **Type**: `integer`
- **Description**: Number of CPU cores
- **Range**: 1-64 (practical limit depends on host)
- **Example**: `2`

### `resources.memory_mb` (required)

- **Type**: `integer`
- **Description**: Memory allocation in megabytes
- **Range**: 128-65536 (practical limit depends on host)
- **Example**: `1024`

---

## Workload Configuration

Describes how to start and interact with your application.

```json
{
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/sbin/nginx", "-g", "daemon off;"],
    "base_url": "http://127.0.0.1:80",
    "workdir": "/",
    "env": {
      "KEY": "value"
    }
  }
}
```

### `workload.type` (required)

- **Type**: `string`
- **Description**: Workload type (affects how Volant interacts with it)
- **Values**:
  - `"http"` — HTTP server (most common)
  - `"tcp"` — TCP service
  - `"daemon"` — Background process
  - `"batch"` — One-time task
- **Example**: `"http"`

### `workload.entrypoint` (required)

- **Type**: `array` of `string`
- **Description**: Command and arguments to start the application
- **Format**: First element is the binary path, rest are arguments
- **Examples**:
  ```json
  ["/usr/sbin/nginx", "-g", "daemon off;"]
  ["/usr/bin/python3", "/app/server.py"]
  ["/bin/sh", "-c", "exec /usr/local/bin/myapp"]
  ```

### `workload.base_url` (conditional)

- **Type**: `string` (URL)
- **Description**: Base URL for HTTP operations (required if `type` is `"http"`)
- **Format**: `http://<ip>:<port>[/path]`
- **Examples**:
  ```json
  "http://127.0.0.1:80"
  "http://0.0.0.0:8080"
  "http://localhost:3000/api"
  ```

### `workload.workdir` (optional)

- **Type**: `string` (path)
- **Description**: Working directory for the entrypoint process
- **Default**: `"/"`
- **Example**: `"/opt/app"`

### `workload.env` (optional)

- **Type**: `object` (key-value pairs)
- **Description**: Environment variables for the workload
- **Example**:
  ```json
  {
    "PATH": "/usr/local/bin:/usr/bin:/bin",
    "LOG_LEVEL": "info",
    "DATABASE_URL": "postgres://localhost/mydb"
  }
  ```

---

## Health Check

Configures how Volant verifies your application is running.

```json
{
  "health_check": {
    "endpoint": "/health",
    "timeout_ms": 10000,
    "interval_ms": 5000,
    "retries": 3
  }
}
```

### `health_check.endpoint` (required)

- **Type**: `string` (path)
- **Description**: HTTP endpoint to check (relative to `workload.base_url`)
- **Examples**: `"/"`, `"/health"`, `"/api/ping"`

### `health_check.timeout_ms` (optional)

- **Type**: `integer`
- **Description**: Maximum time to wait for response (milliseconds)
- **Default**: `10000` (10 seconds)
- **Range**: 1000-60000

### `health_check.interval_ms` (optional)

- **Type**: `integer`
- **Description**: Time between health checks (milliseconds)
- **Default**: `5000` (5 seconds)
- **Range**: 1000-300000

### `health_check.retries` (optional)

- **Type**: `integer`
- **Description**: Number of consecutive failures before marking unhealthy
- **Default**: `3`
- **Range**: 1-10

---

## Network Configuration

Specifies networking mode for the plugin.

```json
{
  "network": {
    "mode": "bridged"
  }
}
```

### `network.mode` (optional)

- **Type**: `string`
- **Description**: Network mode
- **Values**:
  - `"bridged"` (default) — Linux bridge with TAP device and static IP
  - `"vsock"` — Vsock-only (no network, host communication via vsock)
  - `"dhcp"` — Bridge with DHCP (requires DHCP server on bridge)
- **Default**: `"bridged"`

---

## Cloud-Init Configuration

Configures cloud-init support for development environments.

```json
{
  "cloud_init": {
    "datasource": "NoCloud",
    "seed_mode": "disk",
    "user_data": {
      "content": "#cloud-config\nruncmd:\n  - echo 'Hello' > /tmp/hello.txt"
    }
  }
}
```

### `cloud_init.datasource` (optional)

- **Type**: `string`
- **Description**: Cloud-init datasource
- **Value**: `"NoCloud"` (currently the only supported option)
- **Default**: `"NoCloud"`

### `cloud_init.seed_mode` (optional)

- **Type**: `string`
- **Description**: How cloud-init seed is provided
- **Values**:
  - `"disk"` — VFAT disk with label `CIDATA`
  - `"iso"` — ISO9660 image (deprecated)
- **Default**: `"disk"`

### `cloud_init.user_data` (optional)

- **Type**: `object`
- **Description**: Cloud-init user-data configuration
- **Fields**:
  - `content` (string): Inline cloud-config YAML
  - `path` (string): Path to cloud-config file
  - `inline` (boolean): Whether content is inline

**Example**:
```json
{
  "user_data": {
    "content": "#cloud-config\nruncmd:\n  - systemctl start myservice",
    "inline": true
  }
}
```

### `cloud_init.meta_data` (optional)

- **Type**: `object`
- **Description**: Cloud-init meta-data configuration
- **Fields**: Same as `user_data`

### `cloud_init.network_cfg` (optional)

- **Type**: `object`
- **Description**: Cloud-init network configuration
- **Fields**: Same as `user_data`

---

## Additional Disks

Attach additional disk images to the VM.

```json
{
  "disks": [
    {
      "name": "data",
      "source": "/var/lib/volant/data/myapp.img",
      "checksum": "sha256:...",
      "readonly": false
    }
  ]
}
```

### `disks[].name` (required)

- **Type**: `string`
- **Description**: Disk identifier

### `disks[].source` (required)

- **Type**: `string` (path)
- **Description**: Path to disk image file

### `disks[].checksum` (optional)

- **Type**: `string`
- **Description**: SHA256 checksum prefixed with `sha256:`

### `disks[].readonly` (optional)

- **Type**: `boolean`
- **Description**: Whether disk is read-only
- **Default**: `false`

---

## Port Exposure

Define ports to expose from the VM.

```json
{
  "expose": [
    {
      "name": "http",
      "protocol": "tcp",
      "port": 80,
      "host_port": 8080
    }
  ]
}
```

### `expose[].name` (optional)

- **Type**: `string`
- **Description**: Port mapping name

### `expose[].protocol` (optional)

- **Type**: `string`
- **Values**: `"tcp"` or `"udp"`
- **Default**: `"tcp"`

### `expose[].port` (required)

- **Type**: `integer`
- **Description**: Port inside the VM
- **Range**: 1-65535

### `expose[].host_port` (optional)

- **Type**: `integer`
- **Description**: Port on the host (0 = don't forward)
- **Range**: 0-65535
- **Default**: `0`

---

## Validation

Volant validates manifests against the JSON schema. Common errors:

### Missing Required Fields

```
Error: manifest validation failed: "name" is required
```

**Solution**: Add all required fields.

### Invalid Type

```
Error: manifest validation failed: "resources.cpu_cores" must be an integer
```

**Solution**: Check field types match schema.

### Invalid Value

```
Error: manifest validation failed: "schema_version" must be "1.0"
```

**Solution**: Use supported values.

### Conflicting Fields

```
Error: manifest validation failed: cannot specify both "initramfs" and "rootfs"
```

**Solution**: Choose one boot media type.

---

## Minimal Valid Manifests

### Initramfs Plugin

```json
{
  "schema_version": "1.0",
  "name": "myapp",
  "version": "1.0.0",
  "runtime": "myapp",
  "enabled": true,
  
  "initramfs": {
    "url": "/var/lib/volant/plugins/myapp/plugin.cpio.gz",
    "checksum": "sha256:abc123..."
  },
  
  "resources": {
    "cpu_cores": 1,
    "memory_mb": 512
  },
  
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/bin/myapp"],
    "base_url": "http://127.0.0.1:8080"
  },
  
  "health_check": {
    "endpoint": "/"
  }
}
```

### Rootfs Plugin

```json
{
  "schema_version": "1.0",
  "name": "nginx",
  "version": "1.0.0",
  "runtime": "nginx",
  "enabled": true,
  
  "rootfs": {
    "url": "/var/lib/volant/plugins/nginx/rootfs.img",
    "checksum": "sha256:def456...",
    "format": "ext4"
  },
  
  "resources": {
    "cpu_cores": 2,
    "memory_mb": 1024
  },
  
  "workload": {
    "type": "http",
    "entrypoint": ["/usr/sbin/nginx", "-g", "daemon off;"],
    "base_url": "http://127.0.0.1:80"
  },
  
  "health_check": {
    "endpoint": "/"
  }
}
```

---

## JSON Schema

The complete JSON Schema is available at:

**URL**: `https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json`

Use it for:
- **IDE validation** (VSCode, IntelliJ)
- **Automated testing** (CI/CD pipelines)
- **Documentation generation**

**VSCode setup**:

```json
// .vscode/settings.json
{
  "json.schemas": [
    {
      "fileMatch": ["manifest/*.json", "**/manifest.json"],
      "url": "https://raw.githubusercontent.com/volantvm/volant/main/docs/schemas/plugin-manifest-v1.json"
    }
  ]
}
```

---

## Best Practices

1. ✅ **Always include `$schema`** for IDE support
2. ✅ **Use absolute paths** for `url` fields
3. ✅ **Include checksums** for all boot media and disks
4. ✅ **Set realistic resource defaults** (users can override)
5. ✅ **Provide health check endpoints** for reliable orchestration
6. ✅ **Document environment variables** in plugin README
7. ✅ **Version your manifests** alongside plugin artifacts

---

## Next Steps

- **[Plugin Development Overview](../4_plugin-development/1_overview.md)** — Creating plugins
- **[REST API Reference](2_rest-api.md)** — Volant HTTP API
- **[CLI Reference](3_cli-reference.md)** — Complete command reference

---

*Complete manifest schema reference.*
