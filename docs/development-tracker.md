# Viper Development Tracker

| Area | Task | Owner | Status | Notes |
| --- | --- | --- | --- | --- |
| Foundations | Establish roadmap & tracker docs | Codex | Completed | Roadmap + tracker published |
| Tooling | Bootstrap Go modules & repo scaffolding | Codex | Completed | Modules resolved; build/test scaffolding in place |
| Server | Define DB schema & migration strategy | Codex | In Progress | SQLite store, typed repos, CH launcher + bridge networking + evented REST API + auth middleware |
| Image Pipeline | Outline Docker→initramfs workflow | Codex | Completed | Makefile integration complete with dependency checks, artifact verification, and checksums; Docker→initramfs workflow fully operational |
| Agent | Specify chromedp integration contract | Codex | Pending | Await engine interface draft |
| CLI/TUI | Draft command/TUI architecture | Codex | In Progress | CLI vms subcommands + event streaming added |
| Protocols | Design MCP & AG-UI adapters | Codex | Pending | Need event bus contract |
| Installer | Define `viper setup` host workflow | Codex | Pending | Requires orchestrator baseline |

_Last updated: 2025-09-23T14:35:00Z_