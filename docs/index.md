---
title: Viper Documentation
description: |
  Comprehensive documentation for the Viper project
---

# Overview

Welcome to the Viper documentation. This site is the canonical guide for installing, operating, and extending the Viper microVM automation platform.

## Documentation Map

- **[Quickstart](/docs/getting-started/quickstart)**: Provision a Viper host, launch a microVM, and submit your first browser automation task.
- **[Installation](/docs/install)**: Detailed install flows for macOS, Ubuntu, and scripted environments.
- **[Architecture](/docs/architecture)**: Deep-dive into the orchestrator, agent, image pipeline, and networking model.
- **[API Reference](/docs/api-reference)**: REST + agent proxy endpoints, MCP surface, and AG-UI event schema.
- **[Protocols](/docs/protocols)**: MCP and AG-UI specifications, sample payloads, and client integration notes.
- **[CLI Reference](/docs/reference/cli)**: All `viper` commands, flags, and TUI keybindings.
- **[Troubleshooting](/docs/setup/troubleshooting)**: Common failures, diagnostics, and recovery steps.

The sidebar lists every section in reading order; each page has a table of contents for quick navigation.

## Audience

The documentation is segmented for:

- **Operators** who install and maintain a Viper deployment.
- **Developers** who orchestrate browser automation tasks via the CLI or REST API.
- **Integrators** who embed Viper into AI-agent workflows using MCP/AG-UI.
- **Contributors** who build custom images, extensions, or protocol hooks.

## Conventions

- Shell commands use `bash` unless noted.
- File paths are absolute from the Viper repo root unless a page specifies otherwise.
- API examples default to `curl` and JSON. Replace `<PLACEHOLDERS>` before running.

## Need Help?

If you hit an issue, review the troubleshooting guide first. For bugs or feature requests, open an issue in the Viper repo with logs, environment details, and reproduction steps.
