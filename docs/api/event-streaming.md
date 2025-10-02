---
title: "Event Streaming (SSE)"
description: "Consuming the Server-Sent Events (SSE) feed for Volant."
---

# Event Streaming (SSE)

Volant provides a real-time Server-Sent Events (SSE) stream for VM lifecycle data. For agent log streaming and DevTools, see the VM WebSocket endpoints. The legacy AG-UI WebSocket has been removed.

## Server-Sent Events (SSE)

```bash
GET /api/v1/events/vms
Accept: text/event-stream
```

- Emits lifecycle events (`VM_CREATED`, `VM_RUNNING`, `VM_STOPPED`, `VM_LOG`, etc.)
- Each event includes a JSON payload with timestamp, status, and message.
- Clients must handle reconnection logic.

### Example Event
```
event: VM_RUNNING
data: {"type":"VM_RUNNING","name":"demo","status":"running","ip_address":"192.168.127.5","timestamp":"2025-09-24T12:34:56Z","message":"vm running"}
```

### Client Example (JavaScript)
```js
const eventSource = new EventSource('http://127.0.0.1:7777/api/v1/events/vms');

eventSource.addEventListener('VM_CREATED', (event) => {
  const data = JSON.parse(event.data);
  console.log('A new VM was created:', data);
});

eventSource.addEventListener('VM_STOPPED', (event) => {
  const data = JSON.parse(event.data);
  console.log('A VM was stopped:', data);
});

eventSource.onerror = (err) => {
  console.error("EventSource failed:", err);
};
```

## Internal Event Bus

Internally, `internal/server/eventbus` broadcasts `orchestratorevents.VMEvent` objects to all subscribers (SSE, CLI, and MCP).

The internal event payload shape is as follows:

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