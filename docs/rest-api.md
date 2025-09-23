# REST API Reference (Preview)

Base URL defaults to `http://127.0.0.1:7777` (configurable via `VIPER_API_BASE`). Authentication/authorization are development-grade; see `docs/auth.md`.

## Health

### `GET /healthz`
Returns `{ "status": "ok" }` when the service is responsive.

## Virtual Machines

### `GET /api/v1/vms`
Returns a JSON array of VM objects:
```
[
  {
    "id": 1,
    "name": "demo",
    "status": "running",
    "ip_address": "192.168.127.2",
    "mac_address": "02:aa:...",
    "cpu_cores": 2,
    "memory_mb": 2048,
    "kernel_cmdline": "console=ttyS0 ...",
    "pid": 12345
  }
]
```

### `POST /api/v1/vms`
Creates a VM.
```
{
  "name": "demo",
  "cpu_cores": 2,
  "memory_mb": 2048,
  "kernel_cmdline": "optional extra params"
}
```
Responses:
- `201 Created` with VM object on success.
- `409 Conflict` if VM already exists.

### `GET /api/v1/vms/{name}`
Fetch a single VM. Returns `404` if not found.

### `DELETE /api/v1/vms/{name}`
Destroys a VM. Returns `204 No Content` or `404` if missing.

## Events

### `GET /api/v1/events/vms`
Server-Sent Events stream of VM lifecycle changes. Each event has:
```
{
  "type": "VM_RUNNING",
  "name": "demo",
  "status": "running",
  "ip_address": "192.168.127.2",
  "mac_address": "02:..",
  "pid": 12345,
  "timestamp": "2025-09-23T10:00:00Z",
  "message": "vm running"
}
```

Clients should send `Accept: text/event-stream` and handle reconnect logic.

## Errors
Errors return JSON `{ "error": "message" }` with appropriate HTTP status codes (`400`, `401`, `403`, `404`, `409`, `500`).

## Future Additions
- Pagination & filtering for VM list.
- Task, profile, and artifact routes.
- Swagger/OpenAPI description once API is stable.
