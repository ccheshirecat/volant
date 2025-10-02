---
title: "AG-UI (Removed)"
description: "The legacy AG-UI WebSocket endpoint has been removed."
---

# AG-UI Removed

The AG-UI and its `/ws/v1/agui` WebSocket endpoint have been removed. Use the following interfaces instead:

- REST endpoints documented in this section
- CLI commands via `volar`
- MCP endpoint for LLM integrations

Existing third-party clients leveraging AG-UI should migrate to Server-Sent Events (`/api/v1/events/vms`) or poll REST endpoints as appropriate.
