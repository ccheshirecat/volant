---
title: Overhyped Documentation
description: |
  Comprehensive documentation for the Overhyped project
---

# Overview

Welcome to the Overhyped documentation. This site is the canonical guide for installing, operating, and extending the Overhyped microVM automation platform.

## Documentation Map

- **[Quickstart](/docs/getting-started/quickstart)**: Provision an Overhyped host, launch a microVM, and submit your first browser automation task.
- **[Installation](/docs/install)**: Detailed install flows for macOS, Ubuntu, and scripted environments.
- **[Architecture](/docs/architecture)**: Deep-dive into the orchestrator, agent, image pipeline, and networking model.
- **[API Reference](/docs/api-reference)**: REST + agent proxy endpoints, MCP surface, and AG-UI event schema.
- **[Protocols](/docs/protocols)**: MCP and AG-UI specifications, sample payloads, and client integration notes.
- **[CLI Reference](/docs/reference/cli)**: All `hype` commands, flags, and TUI keybindings.
- **[Troubleshooting](/docs/setup/troubleshooting)**: Common failures, diagnostics, and recovery steps.

The sidebar lists every section in reading order; each page has a table of contents for quick navigation.

## Audience

The documentation is segmented for:

- **Operators** who install and maintain a Overhyped deployment.
- **Developers** who orchestrate browser automation tasks via the CLI or REST API.
- **Integrators** who embed Overhyped into AI-agent workflows using MCP/AG-UI.
- **Contributors** who build custom images, extensions, or protocol hooks.

## Conventions

- Shell commands use `bash` unless noted.
- File paths are absolute from the Overhyped repo root unless a page specifies otherwise.
- API examples default to `curl` and JSON. Replace `<PLACEHOLDERS>` before running.

## Need Help?

If you hit an issue, review the troubleshooting guide first. For bugs or feature requests, open an issue in the Overhyped repo with logs, environment details, and reproduction steps.
