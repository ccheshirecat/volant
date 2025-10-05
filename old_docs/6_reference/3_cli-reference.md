# CLI Reference

Complete reference for the `volar` command-line interface.

---

## Table of Contents

- [Global Options](#global-options)
- [vms - Virtual Machine Management](#vms---virtual-machine-management)
- [deployments - Deployment Management](#deployments---deployment-management)
- [plugins - Plugin Management](#plugins---plugin-management)
- [setup - Host Configuration](#setup---host-configuration)
- [version - Version Information](#version---version-information)
- [Environment Variables](#environment-variables)
- [Output Formats](#output-formats)

---

## Global Options

All commands support these global flags:

```
--api, -a <URL>    volantd API base URL (default: http://127.0.0.1:7777)
                   Can also be set via VOLANT_API_BASE environment variable
```

**Example**:

```bash
# Connect to remote volantd instance
volar --api http://volant.example.com:7777 vms list

# Or use environment variable
export VOLANT_API_BASE=http://volant.example.com:7777
volar vms list
```

---

## vms - Virtual Machine Management

Manage microVMs lifecycle and operations.

### volar vms list

List all microVMs.

```bash
volar vms list
```

**Output**:

```
NAME                 STATUS     RUNTIME    IP              MAC                  CPU    MEM
web-1                running    nginx      10.0.0.10       52:54:00:12:34:56    2      512
api-1                running    user-api   10.0.0.11       52:54:00:12:34:57    2      512
db-1                 stopped    postgres   10.0.0.12       52:54:00:12:34:58    4      2048
```

---

### volar vms create

Create a new microVM from a plugin.

```bash
volar vms create <name> [flags]
```

**Flags**:

```
--plugin <name>           Plugin name to use (required if no config file)
--runtime <name>          Override runtime name
--cpu <count>             Number of vCPUs (default: from plugin manifest)
--memory <MB>             Memory in MB (default: from plugin manifest)
--kernel-cmdline <args>   Additional kernel command-line arguments
--api-host <host>         API host for guest agent
--api-port <port>         API port for guest agent
--config <path>           Path to VM config JSON file
```

**Examples**:

```bash
# Create VM from plugin (uses manifest defaults)
volar vms create web-1 --plugin nginx-alpine

# Override resources
volar vms create api-1 --plugin user-api --cpu 4 --memory 1024

# Create from config file
volar vms create db-1 --config database-config.json
```

**Config File Format** (`config.json`):

```json
{
  "plugin": "postgres",
  "runtime": "postgres",
  "manifest": {
    "resources": {
      "vcpu": 4,
      "memory_mb": 2048
    }
  }
}
```

**Notes**:
- If `--config` is provided, it takes precedence over other flags
- The config file can contain either a `vmconfig.Config` object directly or `{"config": {...}}`
- Plugin must be installed before creating VMs

---

### volar vms get

Show detailed information about a microVM.

```bash
volar vms get <name>
```

**Output**:

```
Name: web-1
Status: running
Runtime: nginx-alpine
IP: 10.0.0.10
MAC: 52:54:00:12:34:56
CPU: 2
Memory: 512 MB
PID: 12345
Kernel Cmdline: console=ttyS0 reboot=k panic=1
Serial Socket: /var/run/volant/web-1.serial.sock
Console Socket: /var/run/volant/web-1.console.sock
```

**Example**:

```bash
volar vms get my-vm
```

---

### volar vms start

Start a stopped microVM.

```bash
volar vms start <name>
```

**Example**:

```bash
volar vms start web-1
```

**Errors**:
- `VM not found` if VM doesn't exist
- `VM already running` if VM is already started

---

### volar vms stop

Stop a running microVM gracefully.

```bash
volar vms stop <name> [flags]
```

**Flags**:

```
--force    Force stop without graceful shutdown
```

**Examples**:

```bash
# Graceful stop (sends SIGTERM, waits for shutdown)
volar vms stop web-1

# Force stop (immediate termination)
volar vms stop web-1 --force
```

---

### volar vms restart

Restart a microVM (stop + start).

```bash
volar vms restart <name>
```

**Example**:

```bash
volar vms restart web-1
```

**Note**: Configuration changes may require a restart to take effect.

---

### volar vms delete

Delete a microVM (stops if running, then removes).

```bash
volar vms delete <name> [flags]
```

**Flags**:

```
--force    Force delete without graceful shutdown
```

**Examples**:

```bash
# Graceful delete
volar vms delete web-1

# Force delete
volar vms delete web-1 --force
```

**Warning**: This operation is irreversible. All VM state is lost.

---

### volar vms console

Attach to a microVM's serial console.

```bash
volar vms console <name> [flags]
```

**Flags**:

```
--socket <path>    Override console socket path
```

**Example**:

```bash
volar vms console web-1
```

**Usage**:
- Press `Ctrl+C` to detach from console
- Console shows kernel boot messages and serial output
- Useful for debugging boot issues

**Note**: Terminal is set to raw mode for proper interaction.

---

### volar vms config

Get or update VM configuration.

#### Get Configuration

```bash
volar vms config get <name>
```

**Output** (JSON):

```json
{
  "resources": {
    "vcpu": 2,
    "memory_mb": 512
  },
  "network": {
    "mode": "bridge",
    "bridge_name": "volbr0"
  }
}
```

#### Update Configuration

```bash
volar vms config update <name> [flags]
```

**Flags**:

```
--cpu <count>      Update vCPU count
--memory <MB>      Update memory size
```

**Example**:

```bash
# Update resources (requires restart)
volar vms config update web-1 --cpu 4 --memory 1024

# Restart to apply changes
volar vms restart web-1
```

---

### volar vms scale

Scale a single VM's resources (alias for `vms config update`).

```bash
volar vms scale <name> --cpu <count> --memory <MB>
```

**Example**:

```bash
volar vms scale web-1 --cpu 4 --memory 1024
volar vms restart web-1
```

---

### volar vms operations

List available plugin operations for a VM (from OpenAPI spec).

```bash
volar vms operations <name>
```

**Output**:

```
OPERATION ID                   METHOD   PATH                                     SUMMARY
------------------------------------------------------------------------------------------------------------------------
getStatus                      GET      /api/status                              Get application status
updateConfig                   POST     /api/config                              Update configuration
performAction                  POST     /api/actions/{action}                    Trigger an action
```

**Example**:

```bash
volar vms operations web-1
```

**Use Case**: Discover what operations a plugin exposes before calling them.

---

### volar vms call

Invoke a plugin operation dynamically via OpenAPI spec.

```bash
volar vms call <vm> <operation-id> [flags]
```

**Flags**:

```
--query <key=value>      Query parameter (repeatable)
--body <json>            Inline request body (JSON)
--body-file <path>       Path to request body file
--timeout <duration>     Request timeout (default: 60s)
```

**Examples**:

```bash
# Call operation by ID
volar vms call web-1 getStatus

# Call operation by METHOD:PATH
volar vms call web-1 POST:/api/config --body '{"log_level":"debug"}'

# Call with query parameters
volar vms call api-1 searchUsers --query name=john --query limit=10

# Call with body from file
volar vms call api-1 updateConfig --body-file config.json
```

**Output**:

```
HTTP 200
{
  "status": "healthy",
  "uptime": 3600
}
```

**Notes**:
- Requires plugin to expose OpenAPI spec
- Automatically proxies requests through volantd to VM
- Supports all HTTP methods (GET, POST, PUT, PATCH, DELETE)

---

## deployments - Deployment Management

Manage multi-replica deployments (Kubernetes-style ReplicaSets).

### volar deployments list

List all deployments.

```bash
volar deployments list
```

**Output**:

```
NAME         PLUGIN       REPLICAS   READY
api          user-api     3          3
web          nginx        5          5
worker       batch-job    2          1
```

**Example**:

```bash
volar deployments list
```

---

### volar deployments create

Create a new deployment.

```bash
volar deployments create <name> [flags]
```

**Flags**:

```
--config <path>       Path to deployment config JSON file (required)
--replicas <count>    Number of replicas (default: 1)
```

**Examples**:

```bash
# Create deployment from config file
volar deployments create api --config api-deployment.json --replicas 3
```

**Config File Format** (`api-deployment.json`):

```json
{
  "plugin": "user-api",
  "runtime": "user-api"
}
```

Or with full VM config:

```json
{
  "plugin": "user-api",
  "runtime": "user-api",
  "manifest": {
    "resources": {
      "vcpu": 2,
      "memory_mb": 512
    }
  }
}
```

**Output**:

```
Deployment api created with 3 replicas
```

**Notes**:
- Creates VMs named `<deployment>-<index>` (e.g., `api-1`, `api-2`, `api-3`)
- All replicas use the same plugin and config
- Automatically manages replica lifecycle

---

### volar deployments get

Show detailed deployment information.

```bash
volar deployments get <name> [flags]
```

**Flags**:

```
--output <path>    Write deployment details to file
```

**Output** (JSON):

```json
{
  "name": "api",
  "desired_replicas": 3,
  "ready_replicas": 3,
  "vms": ["api-1", "api-2", "api-3"],
  "config": {
    "plugin": "user-api",
    "runtime": "user-api"
  }
}
```

**Examples**:

```bash
# Display deployment details
volar deployments get api

# Save to file
volar deployments get api --output api-state.json
```

---

### volar deployments scale

Scale deployment replicas up or down.

```bash
volar deployments scale <name> <replicas>
```

**Examples**:

```bash
# Scale up to 10 replicas
volar deployments scale api 10

# Scale down to 2 replicas
volar deployments scale api 2

# Scale to 0 (stop all replicas)
volar deployments scale api 0
```

**Output**:

```
Deployment api scaled to 10 replicas (ready 3)
```

**Notes**:
- Scaling up creates new VMs instantly
- Scaling down gracefully stops excess VMs
- Replica names are deterministic: `<deployment>-<index>`

---

### volar deployments delete

Delete a deployment and all its VMs.

```bash
volar deployments delete <name>
```

**Example**:

```bash
volar deployments delete api
```

**Output**:

```
Deployment api deleted
```

**Warning**: This stops and deletes all replica VMs. Cannot be undone.

---

## plugins - Plugin Management

Manage plugin manifests.

### volar plugins list

List all installed plugins.

```bash
volar plugins list
```

**Output**:

```
NAME                 VERSION    ENABLED  RUNTIME
nginx-alpine         1.0.0      true     nginx-alpine
user-api             2.1.0      true     user-api
postgres             16.1.0     true     postgres
legacy-app           0.5.0      false    legacy
```

**Example**:

```bash
volar plugins list
```

---

### volar plugins show

Show detailed plugin manifest.

```bash
volar plugins show <name>
```

**Output** (JSON):

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
  "rootfs": {
    "format": "qcow2",
    "size_mb": 150
  }
}
```

**Example**:

```bash
volar plugins show nginx-alpine
```

---

### volar plugins install

Install a plugin from a manifest file or URL.

```bash
volar plugins install [manifest] [flags]
```

**Flags**:

```
--manifest <path>    Path to manifest JSON file
--url <URL>          URL to manifest JSON
```

**Examples**:

```bash
# Install from local file
volar plugins install manifest.json

# Install from URL
volar plugins install https://plugins.example.com/nginx-1.0.0.json

# Using flags explicitly
volar plugins install --manifest manifest.json
volar plugins install --url https://plugins.example.com/nginx.json
```

**Manifest Format** (see [Manifest Schema Reference](1_manifest-schema.md)):

```json
{
  "name": "my-app",
  "version": "1.0.0",
  "description": "My application",
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

**Notes**:
- Plugin artifacts (kernel, rootfs) are downloaded during installation
- Installation validates manifest schema
- Fails if plugin name already exists

---

### volar plugins remove

Remove an installed plugin.

```bash
volar plugins remove <name>
```

**Example**:

```bash
volar plugins remove nginx-alpine
```

**Errors**:
- `Plugin not found` if plugin doesn't exist
- `Plugin in use` if VMs are currently using this plugin

**Note**: Must stop all VMs using the plugin before removal.

---

### volar plugins enable

Enable a disabled plugin.

```bash
volar plugins enable <name>
```

**Example**:

```bash
volar plugins enable legacy-app
```

**Effect**: Allows creating new VMs from this plugin.

---

### volar plugins disable

Disable a plugin (prevents new VMs).

```bash
volar plugins disable <name>
```

**Example**:

```bash
volar plugins disable legacy-app
```

**Effect**:
- Prevents creating new VMs from this plugin
- Existing VMs continue to run
- Useful for deprecating old plugin versions

---

## setup - Host Configuration

Helper commands for configuring the host system.

### volar setup bridge

Configure bridge networking for Volant.

```bash
volar setup bridge [flags]
```

**Flags**:

```
--bridge-name <name>    Bridge name (default: volbr0)
--subnet <cidr>         Bridge subnet (default: 10.0.0.0/24)
--help                  Show help
```

**Example**:

```bash
# Setup default bridge
sudo volar setup bridge

# Custom bridge configuration
sudo volar setup bridge --bridge-name mybr0 --subnet 192.168.100.0/24
```

**Actions Performed**:
1. Creates bridge interface
2. Assigns IP address
3. Enables IP forwarding
4. Configures iptables NAT rules
5. Persists configuration across reboots (systemd-networkd)

**Requirements**: Must run with root privileges (`sudo`)

---

### volar setup systemd

Install volantd as a systemd service.

```bash
volar setup systemd [flags]
```

**Flags**:

```
--volantd-path <path>    Path to volantd binary (default: /usr/local/bin/volantd)
--user <name>            User to run service as (default: root)
--help                   Show help
```

**Example**:

```bash
# Install with defaults
sudo volar setup systemd

# Custom configuration
sudo volar setup systemd --volantd-path /opt/volant/volantd --user volant
```

**Actions Performed**:
1. Creates systemd unit file at `/etc/systemd/system/volantd.service`
2. Enables service to start on boot
3. Starts the service immediately

**Service Management**:

```bash
# Check status
sudo systemctl status volantd

# View logs
sudo journalctl -u volantd -f

# Restart service
sudo systemctl restart volantd
```

---

## version - Version Information

Display volar CLI version.

```bash
volar version
```

**Output**:

```
volar CLI (prototype)
```

**Example**:

```bash
volar version
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `VOLANT_API_BASE` | volantd API base URL | `http://127.0.0.1:7777` |

**Example**:

```bash
# Connect to remote volantd
export VOLANT_API_BASE=http://volant-prod.example.com:7777
volar vms list
```

---

## Output Formats

### Table Format (Default)

Most list commands output in table format:

```bash
volar vms list
```

```
NAME                 STATUS     RUNTIME    IP              MAC                  CPU    MEM
web-1                running    nginx      10.0.0.10       52:54:00:12:34:56    2      512
```

### JSON Format

Detail commands output JSON:

```bash
volar vms get web-1
```

```json
{
  "name": "web-1",
  "status": "running",
  "runtime": "nginx-alpine",
  "ip": "10.0.0.10"
}
```

### Parsing JSON with jq

```bash
# Extract IP address
volar vms get web-1 | jq -r .ip

# List all running VMs (requires API call)
curl http://localhost:7777/api/v1/vms | jq -r '.[] | select(.status=="running") | .name'

# Get VM count
curl http://localhost:7777/api/v1/vms | jq 'length'
```

---

## Common Workflows

### Create and Start a VM

```bash
# Install plugin
volar plugins install https://plugins.volantvm.com/nginx-1.0.0.json

# Create VM
volar vms create web-1 --plugin nginx-alpine

# Check status
volar vms get web-1

# Access VM
curl http://$(volar vms get web-1 | grep IP | awk '{print $2}')
```

### Scale a Deployment

```bash
# Create deployment
cat > api-deployment.json <<EOF
{
  "plugin": "user-api",
  "runtime": "user-api"
}
EOF

volar deployments create api --config api-deployment.json --replicas 3

# Scale up
volar deployments scale api 10

# Check status
volar deployments get api

# Scale down
volar deployments scale api 2
```

### Invoke Plugin Operations

```bash
# Discover available operations
volar vms operations web-1

# Call an operation
volar vms call web-1 getStatus

# Call with parameters
volar vms call api-1 searchUsers --query name=john --query limit=10

# Call with request body
volar vms call api-1 updateConfig --body '{"log_level":"debug"}'
```

### Debug VM Issues

```bash
# Check VM status
volar vms get my-vm

# Attach to console (see boot messages)
volar vms console my-vm

# Restart VM
volar vms restart my-vm

# Force stop if hanging
volar vms stop my-vm --force
```

---

## Error Messages

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `connection refused` | volantd not running | Start volantd: `sudo systemctl start volantd` |
| `VM not found` | VM doesn't exist | Check VM name: `volar vms list` |
| `Plugin not found` | Plugin not installed | Install plugin: `volar plugins install ...` |
| `Plugin in use` | VMs using plugin | Stop VMs first: `volar vms delete <name>` |
| `VM already running` | VM already started | Check status: `volar vms get <name>` |
| `permission denied` | Insufficient privileges | Run with `sudo` for setup commands |

---

## Shell Completion

Generate shell completion scripts:

**Bash**:

```bash
volar completion bash > /etc/bash_completion.d/volar
```

**Zsh**:

```bash
volar completion zsh > ~/.zsh/completion/_volar
```

**Fish**:

```bash
volar completion fish > ~/.config/fish/completions/volar.fish
```

*(Note: Completion commands may not be implemented yet)*

---

## Next Steps

- **[REST API Reference](2_rest-api.md)** – HTTP API documentation
- **[Manifest Schema](1_manifest-schema.md)** – Plugin manifest specification
- **[Getting Started Guide](../2_getting-started/1_installation.md)** – Installation and setup
- **[Plugin Development](../4_plugin-development/1_overview.md)** – Build custom plugins
