---
title: "Model Context Protocol"
description: "Integrating Viper with LLM agents through the MCP endpoint."
---

# Model Context Protocol (MCP)

Viper exposes a Model Context Protocol endpoint to enable LLM-driven orchestration flows.

## Endpoint

```
POST /api/v1/mcp
Content-Type: application/json
```

Request payload:

```json
{
  "command": "viper.vms.list",
  "params": {}
}
```

Response payload:

```json
{
  "result": [ ... ],
  "error": ""
}
```

- `result` — command-specific response (nullable)
- `error` — string containing error message (if any)

## Supported Commands

| Command | Description | Parameters | Response |
| --- | --- | --- | --- |
| `viper.vms.list` | List all orchestrated VMs | none | Array of VM summaries (id, name, status, ip, cpu, memory) |
| `viper.vms.create` | Create a VM with default resources | `name` (string) | VM summary |
| `viper.system.get_capabilities` | Discover available commands | none | Command metadata (name, description, params) |

## Command Examples

### List VMs

Request:
```json
{"command": "viper.vms.list", "params": {}}
```
Response (success):
```json
{
  "result": [
    {
      "id": 5,
      "name": "demo",
      "status": "running",
      "ip_address": "192.168.127.5",
      "cpu_cores": 2,
      "memory_mb": 2048
    }
  ]
}
```

### Create VM

Request:
```json
{
  "command": "viper.vms.create",
  "params": {
    "name": "demo"
  }
}
```

Response on success:
```json
{
  "result": {
    "id": 6,
    "name": "demo",
    "status": "pending",
    "ip_address": "192.168.127.6",
    "cpu_cores": 2,
    "memory_mb": 2048
  }
}
```

Response on error:
```json
{
  "error": "vm demo already exists"
}
```

## Error Handling

- Invalid command → `error" = "unknown command: ..."`
- Missing parameters → `error" = "name param required"`
- Server-side failure → `error" = <engine error message>`

## Extensibility

Add new MCP commands by extending the switch in `internal/server/httpapi/httpapi.go` (`handleMCP`). Follow the pattern of returning structured results and setting `error` when failures occur.
