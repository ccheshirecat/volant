# Viper Development Tracker

| Area | Task | Owner | Status | Notes |
| --- | --- | --- | --- | --- |
| Foundations | Establish roadmap & tracker docs | Codex | Completed | Roadmap + tracker published |
| Tooling | Bootstrap Go modules & repo scaffolding | Codex | Completed | Modules resolved; build/test scaffolding in place |
| Server | Define DB schema & migration strategy | Codex | In Progress | SQLite store, typed repos, CH launcher + bridge networking + evented REST API + auth middleware |
| Image Pipeline | Outline Docker→initramfs workflow | Codex | Completed | Makefile integration complete with dependency checks, artifact verification, and checksums; Docker→initramfs workflow fully operational |
| Agent | Specify chromedp integration contract | Codex | In Progress | chromedp v0.14 API migration underway; runtime upgraded with navigation listeners and cookie/storage helpers; remaining task: clean up handler usage in app package |
| CLI/TUI | Draft command/TUI architecture | Codex | In Progress | CLI vms subcommands + event streaming added; DevTools proxy HTTP/WS path implemented; TUI command queue wired |
| Protocols | Design MCP & AG-UI adapters | Codex | Pending | Need event bus contract |
| Installer | Define `viper setup` host workflow | Codex | Pending | Requires orchestrator baseline |

_Last updated: 2025-09-24T12:15:00Z_