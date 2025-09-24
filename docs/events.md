---
title: "Event Streaming"
description: "SSE and WebSocket event feeds for Overhyped."
---

# Event Streaming

Overhyped provides multiple real-time feeds for lifecycle and log data.

## Server-Sent Events (SSE)

```
GET /api/v1/events/vms
Accept: text/event-stream
```

- Emits lifecycle events (`VM_CREATED`, `VM_RUNNING`, `VM_STOPPED`, `VM_LOG`, etc.)
- Each event includes JSON payload with timestamp, status, and message.
- Clients must handle reconnection.

Example event:
```
event: VM_RUNNING
data: {"type":"VM_RUNNING","name":"demo","status":"running","ip_address":"192.168.127.5","timestamp":"2025-09-24T12:34:56Z","message":"VM running"}
```

## WebSocket Streams

### AG-UI Events

```
GET /ws/v1/agui
```

- Streams AG-UI compatible events (`run_started`, `text`, `run_finished`).
- See [AG-UI protocol](../protocols/agui.md).

### Agent Logs

```
GET /ws/v1/vms/{name}/logs
```

- JSON messages with `name`, `stream`, `line`, `timestamp`.
- Backed by agent `/v1/logs/stream` SSE.

### DevTools Proxy

```
GET /ws/v1/vms/{name}/devtools/
```

- Proxies Chrome DevTools Protocol WebSocket to local clients.
- Use with `hype browsers proxy` for local debugging.

## Event Bus

Internally, `internal/server/eventbus` broadcasts `orchestratorevents.VMEvent` objects to SSE, AG-UI, CLI/TUI, and MCP integrations.

The event payload shape:

```json
{
  "type": "VM_CREATED",
  "name": "demo",
  "status": "pending",
  "ip_address": "192.168.127.5",
  "mac_address": "02:..",
  "timestamp": "2025-09-24T12:34:56Z",
  "message": "VM created",
  "stream": "",
  "line": ""
}
```

Consumers should parse `type` and `status` to update UI or trigger automation.
