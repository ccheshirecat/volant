---
title: "CLI & TUI"
description: "Using the viper command-line interface and interactive TUI."
---

# CLI & TUI Reference

## Overview

`viper` is a dual-mode binary:

- **Command-line** (Cobra commands for scripting)
- **Interactive TUI** (Bubble Tea dashboard)

## Basic Commands

```bash
viper --help
viper vms list
viper vms create my-vm --cpu 2 --memory 2048
viper vms delete my-vm
viper setup --dry-run
```

### Global Flags

- `--api`, `-a`: Override server base URL (`VIPER_API_BASE`)
- `--help`: Show usage

## `viper vms`

| Subcommand | Description | Examples |
| --- | --- | --- |
| `list` | List VMs | `viper vms list` |
| `get <name>` | Show VM details | `viper vms get demo` |
| `create <name>` | Create VM | `viper vms create demo --cpu 4 --memory 4096 --kernel-cmdline "console=ttyS0"` |
| `delete <name>` | Destroy VM | `viper vms delete demo` |

Flags for `create`:
- `--cpu`, `--memory`
- `--kernel-cmdline`

## `viper setup`

See [Installation](../setup/installer.md). Useful flags:

- `--bridge`, `--subnet`, `--host-ip`
- `--runtime-dir`, `--log-dir`
- `--kernel`, `--initramfs`
- `--service-file`
- `--dry-run`

## `viper browsers proxy`

Expose remote Chrome DevTools locally:

```bash
viper browsers proxy demo --port 9223
open http://localhost:9223/json/version
```

## Interactive TUI

Launch by running `viper` without arguments.

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

- `VIPER_API_BASE`: Base URL
- `VIPER_BRIDGE`, `VIPER_SUBNET`, etc. for setup defaults
- `VIPER_KERNEL`, `VIPER_INITRAMFS` for setup service template

## Troubleshooting

- `viper --api http://host:port` if server runs remotely
- Check `~/.viper/logs/` or `journalctl -u viper-server`
- Ensure bridge (`ip link show viperbr0`) and NAT rules exist
