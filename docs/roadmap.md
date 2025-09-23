# Viper v2.0 Implementation Roadmap

## Guiding Principles
- Deliver a production-grade experience end-to-end; no throwaway scaffolding.
- Prioritize deterministic automation; every workflow must be reproducible by `viper setup` and the REST APIs.
- Cloud Hypervisor, static networking, and the embedded SQLite store are non-negotiable foundations.
- Treat the orchestration engine as the source of truth; all other surfaces (CLI, TUI, MCP, AG-UI) consume its APIs.
- Keep surface area focused: fewer features executed impeccably over breadth with compromise.

## Phase 0 – Environment & Project Foundations
- ✅ Repository structure (server, agent, cli, image build, installer) with Go modules and shared tooling in place.
- ✅ Make targets and baseline lint/test scripts defined.
- 📄 Contribution, coding conventions, and CI scaffolding still to be authored.

## Phase 1 – Orchestrator Core (viper-server)
- ✅ Implement embedded SQLite schema migrations (`vms`, `ip_allocations`, `workloads`, `plugins`).
- ✅ Build orchestration services: IP leasing, MAC generation, VM lifecycle, child-process supervision.
- ✅ Expose REST API v1 (VM CRUD, IP pool introspection, health, agent proxy).
- ✅ Integrate event bus feeding REST responses, MCP, AG-UI, and TUI.

## Phase 2 – Image Pipeline & In-VM Stack
- ✅ Create Docker-to-initramfs Make pipeline producing kernel + initramfs artifacts.
- Implement `/bin/viper-init` (mounts, parse cmdline, static networking, exec `viper-agent`).
- Build `viper-agent` Go service wrapping chromedp/headless-shell with REST+CDP proxy surfaces.
- Deliver artifact publishing flow consumable by orchestrator and installer.

## Phase 3 – Client Surfaces
- ✅ Standard CLI (Cobra) covering orchestrator operations, profile management, artifact retrieval.
- ✅ TUI (Bubble Tea) with live dashboards, command palette, websocket-driven updates.
- Browser proxy command bridging CDP WebSocket to localhost.

## Phase 4 – AI-Native Protocols
- MCP endpoint mapping commands -> orchestrator engine with capabilities discovery.
- AG-UI WebSocket emitter streaming orchestrator, agent, and workload events.

## Phase 5 – Installer & Setup Experience
- Author `install.viper.dev` bootstrapper and `viper setup` automation.
- Implement host validation, dependency installation, bridge/NAT setup, systemd service management.
- Provide rollback/recovery and diagnostics commands.

## Phase 6 – Hardening & Release Readiness
- Comprehensive integration tests (VM lifecycle, agent flows, networking, MCP/AG-UI).
- Load/chaos testing for orchestrator stability.
- Security review: privilege boundaries, input validation, TLS/auth (scope TBD).
- Documentation polish: operator guide, admin manual, API references.

## Dependencies & Sequencing Notes
- Orchestrator schema must stabilize before CLI/TUI client development.
- Image pipeline artifacts are prerequisites for end-to-end VM lifecycle tests.
- Event bus contract must be defined ahead of TUI and AG-UI work to avoid rework.
- Installer depends on orchestrator + image artifact availability + systemd unit definitions.

## Sprint 1 (Completed)
1. ✅ Implement typed repositories for VMs and IP allocations atop the SQLite store.
2. ✅ Flesh out orchestrator engine interfaces, including IP lease workflow, Cloud Hypervisor launch integration, and initial crash monitoring.
3. ✅ Start authoring ADRs covering networking model, process supervision (event emission, retries), and event bus contract.
4. ✅ Introduce CI placeholder (lint/test) and contribution guidelines.
5. ✅ Design REST authentication/authorization model and CLI alignment.

## Sprint 2 (Next)
1. Implement browser automation suite in viper-agent (chromedp integration, DOM actions, profiles).
2. Add profile attach/extract commands to CLI and REST API.
3. Enhance TUI with VM selection for logs/screenshots.
4. Begin MCP/AG-UI full integration with more commands/events.