---
title: "CLI & TUI"
description: "Using the volar command-line interface and interactive TUI."
---

# CLI & TUI Reference

## Overview

`volar` is a dual-mode binary:

- **Command-line** (Cobra commands for scripting)
- **Interactive TUI** (Bubble Tea dashboard)

## Basic Commands

```bash
volar --help
volar vms list
volar vms create my-vm --plugin browser --cpu 2 --memory 2048
volar vms delete my-vm
volar setup --dry-run
volar plugins list
```

### Global Flags

- `--api`, `-a`: Override server base URL (`VOLANT_API_BASE`)
- `--help`: Show usage

## `volar vms`

Lifecycle and shortcut automation commands:

| Subcommand | Description | Examples |
| --- | --- | --- |
| `list` | List VMs | `volar vms list` |
| `get <name>` | Show VM details | `volar vms get demo` |
| `create <name>` | Create VM | `volar vms create demo --plugin browser --cpu 4 --memory 4096 --kernel-cmdline "console=ttyS0"` |
| `delete <name>` | Destroy VM | `volar vms delete demo` |
| `navigate <name> <url>` | Open a URL inside the VM’s browser | `volar vms navigate demo https://example.com` |
| `screenshot <name>` | Capture a screenshot via the agent | `volar vms screenshot demo --full-page --output=demo.png` |
| `scrape <name>` | Extract text or an attribute with a CSS selector | `volar vms scrape demo --selector="a.cta" --attr=href` |
| `exec <name>` | Evaluate JavaScript in the page context | `volar vms exec demo -e "document.title"` |
| `graphql <name>` | Execute an authenticated GraphQL request | `volar vms graphql demo --endpoint=https://site/graphql --query='query{viewer{email}}'` |

Common flags:

- `create`: `--plugin` (required), `--cpu`, `--memory`, `--kernel-cmdline`
- `navigate`, `screenshot`, `scrape`, `exec`, `graphql`: `--timeout` to control agent-side duration (default 60s)
- `screenshot`: `--full-page`, `--format`, `--output`
- `scrape`: `--selector`, `--attr`
- `exec`: `--expression`, `--await`
- `graphql`: `--endpoint`, `--query`, `--variables`

## `volar setup`

See [Installation](../setup/installer.md). Useful flags:

- `--bridge`, `--subnet`, `--host-ip`
- `--runtime-dir`, `--log-dir`
- `--kernel`, `--initramfs`
- `--service-file`
- `--dry-run`

## `volar plugins`

Manage runtime manifests handled by the engine:

```bash
volar plugins list
volar plugins show browser
volar plugins install --manifest ./browser.manifest.json
volar plugins enable browser
volar plugins disable browser
volar plugins remove browser
```

Manifests describe runtime metadata (resources, actions, optional OpenAPI specs). See the [Plugins guide](plugins.md) for authoring details.

## `volar browsers`

Browser-specific subcommands moved to the browser plugin repository. The engine CLI retains a stub that directs operators to install the plugin CLI and manifests, but routine workflows should use `volar plugins ...` or runtime-specific tooling packaged with the plugin.

## Interactive TUI

Launch by running `volar` without arguments.

### Layout

- Header: health/status
- VM list pane
- Log pane (SSE stream)
- Command input with history/autocomplete

### Key Bindings

| Key | Action |
| --- | --- |
| `tab` | Cycle focus (VM list ↔ logs ↔ input) /
| `enter` | Execute command or switch to input |
| `ctrl+w` | Clear input |
| `↑`/`↓` | Navigate history or list |
| `q` / `ctrl+c` | Quit |

### Commands

- `help` — show help message
- `status` — refresh status summary
- `vms list`, `vms get`, `vms create`, `vms delete`

### Autocomplete

- Tab completes root commands and VM names.
- Feedback messages appear if no suggestions are available.

### Logs

- Shows recent events/log entries (newest at top)
- `vm logs` stream is appended via SSE

## Environment Variables

- `VOLANT_API_BASE`: Base URL
- `VOLANT_BRIDGE`, `VOLANT_SUBNET`, etc. for setup defaults
- `VOLANT_KERNEL`, `VOLANT_INITRAMFS` for setup service template

## Troubleshooting

- `volar --api http://host:port` if server runs remotely
- Check `~/.volant/logs/` or `journalctl -u volantd`
- Ensure bridge (`ip link show vbr0`) and NAT rules exist
