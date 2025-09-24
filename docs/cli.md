---
title: "CLI & TUI"
description: "Using the hype command-line interface and interactive TUI."
---

# CLI & TUI Reference

## Overview

`hype` is a dual-mode binary:

- **Command-line** (Cobra commands for scripting)
- **Interactive TUI** (Bubble Tea dashboard)

## Basic Commands

```bash
hype --help
hype vms list
hype vms create my-vm --cpu 2 --memory 2048
hype vms delete my-vm
hype setup --dry-run
```

### Global Flags

- `--api`, `-a`: Override server base URL (`OVERHYPED_API_BASE`)
- `--help`: Show usage

## `hype vms`

| Subcommand | Description | Examples |
| --- | --- | --- |
| `list` | List VMs | `hype vms list` |
| `get <name>` | Show VM details | `hype vms get demo` |
| `create <name>` | Create VM | `hype vms create demo --cpu 4 --memory 4096 --kernel-cmdline "console=ttyS0"` |
| `delete <name>` | Destroy VM | `hype vms delete demo` |

Flags for `create`:
- `--cpu`, `--memory`
- `--kernel-cmdline`

## `hype setup`

See [Installation](../setup/installer.md). Useful flags:

- `--bridge`, `--subnet`, `--host-ip`
- `--runtime-dir`, `--log-dir`
- `--kernel`, `--initramfs`
- `--service-file`
- `--dry-run`

## `overhyped browsers proxy`

Expose remote Chrome DevTools locally:

```bash
hype browsers proxy demo --port 9223
open http://localhost:9223/json/version
```

## Interactive TUI

Launch by running `hype` without arguments.

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

- `OVEERHYPED_API_BASE`: Base URL
- `OVERHYPED_BRIDGE`, `OVERHYPED_SUBNET`, etc. for setup defaults
- `OVERHYPED_KERNEL`, `OVERHYPED_INITRAMFS` for setup service template

## Troubleshooting

- `hype --api http://host:port` if server runs remotely
- Check `~/.overhyped/logs/` or `journalctl -u hyped`
- Ensure bridge (`ip link show hypebr0`) and NAT rules exist
