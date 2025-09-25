---
title: "REST API"
description: "Overview of the Volant REST interface and references to the OpenAPI document."
---

# REST API

The Volant control plane exposes a JSON/HTTP API for VM lifecycle management, event streaming, and agent proxying.

## Base URL

- Default: `http://127.0.0.1:7777`
- Override: `VOLANT_API_BASE` environment variable / CLI `--api` flag

## Authentication

- Development builds: unauthenticated (subject to `VOLANT_API_ALLOW_CIDR` whitelist)
- Production: configure `VOLANT_API_KEY` for API key headers (WIP)

## OpenAPI Specification

For full reference, embed `docs/api/openapi.yaml` in your docs site. Highlights:

| Endpoint | Description |
| --- | --- |
| `GET /healthz` | Health check |
| `GET /api/v1/system/status` | VM count + resource usage |
| `GET /api/v1/vms` | List all VMs |
| `POST /api/v1/vms` | Create VM |
| `GET /api/v1/vms/{name}` | Retrieve VM |
| `DELETE /api/v1/vms/{name}` | Destroy VM |
| `GET /api/v1/events/vms` | Server-Sent Events (lifecycle) |
| `POST /api/v1/mcp` | Model Context Protocol commands |
| `ANY /api/v1/vms/{name}/agent/*` | Proxy to agent inside VM |
| `GET /ws/v1/vms/{name}/devtools/*` | CDP proxy |
| `GET /ws/v1/vms/{name}/logs` | WebSocket JSON log stream |
| `GET /ws/v1/agui` | AG-UI WebSocket events (run state) |
| `POST /api/v1/vms/{name}/actions/*` | Shortcut action shims (navigate, screenshot, scrape, exec, graphql) |

## Environment Variables

- `VOLANT_API_BASE`: Override server base URL (client only)
- `VOLANT_API_KEY`: API token
- `VOLANT_API_ALLOW_CIDR`: Comma-separated CIDR whitelist for REST access
- `VOLANT_KERNEL`, `VOLANT_INITRAMFS`: Default kernel artifacts for service templates

## HTTP Status Codes

- `200 OK`: Successful requests
- `201 Created`: VM created
- `204 No Content`: VM deleted
- `400 Bad Request`: Input validation error
- `401 Unauthorized`: Invalid/missing API key
- `403 Forbidden`: CIDR/IP restricted
- `404 Not Found`: VM missing
- `409 Conflict`: VM already exists
- `500 Internal Server Error`: Server failure (check logs)
- `502 Bad Gateway`: Agent unreachable (bridge/network)

## Event Streaming

`GET /api/v1/events/vms` returns Server-Sent Events with JSON payloads:

```
event: VM_RUNNING
data: {"type":"VM_RUNNING","name":"demo","status":"running","timestamp":"2025-09-24T12:34:56Z"}
```

Clients should reconnect on stream closure using `Last-Event-ID`/`EventSource`.

## Rate Limits

None enforced currently; expected to be behind private networks.

## Future Work

- Authentication hardening (mTLS, JWT, OAuth)
- Pagination/filtering for VM list
- Artifact/task endpoints
- Swagger UI bundling in docs site
