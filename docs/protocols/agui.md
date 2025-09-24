---
title: "AG-UI Stream"
description: "Consuming Viper's AG-UI WebSocket event stream."
---

# AG-UI Protocol

Viper emits AG-UI compliant events over a WebSocket to support UI dashboards and agent tooling.

## Endpoint

```
GET /ws/v1/agui
```

- WebSocket upgrade required.
- No authentication in development builds; production should sit behind auth proxy.

## Message Types

The stream contains JSON objects mapped from internal VM events to AG-UI schemas:

### RunStartedEvent

```json
{
  "type": "run_started",
  "id": "vm-name",
  "name": "VM vm-name started"
}
```

### TextMessageEvent

```json
{
  "type": "text",
  "text": "VM vm-name is running"
}
```

### RunFinishedEvent

```json
{
  "type": "run_finished",
  "output": "VM vm-name stopped"
}
```

## Lifecycle Mapping

| Internal VM Event | AG-UI Message |
| --- | --- |
| `VM_CREATED` | `run_started` |
| `VM_RUNNING` | `text` (“VM X is running”) |
| `VM_STOPPED` | `run_finished` |
| `VM_CRASHED` | `run_finished` (future work: include error info) |
| `VM_LOG` | (not broadcast on AG-UI; use log WebSocket) |

## Client Example (JavaScript)

```js
const socket = new WebSocket('ws://127.0.0.1:7777/ws/v1/agui');

socket.onmessage = (event) => {
  const data = JSON.parse(event.data);
  switch (data.type) {
    case 'run_started':
      console.log('VM started:', data.name);
      break;
    case 'text':
      console.log('Message:', data.text);
      break;
    case 'run_finished':
      console.log('VM finished:', data.output);
      break;
    default:
      console.log('Unknown AG-UI event', data);
  }
};
```

## Reconnection Strategy

Implement exponential backoff and replay if needed. Events are stateless; the client should request current VM state via REST if a connection drops.

## Debugging

- Use `wscat` or `websocat` to inspect the stream manually.
- Ensure the event bus is enabled (`viper-server` logs success on startup).
- Check `/api/v1/events/vms` SSE stream as another source of lifecycle events.
