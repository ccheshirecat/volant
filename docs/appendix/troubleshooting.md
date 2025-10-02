---
title: "Troubleshooting"
description: "Common issues and fixes for Volant installations."
---

# Troubleshooting

## `volar setup` errors

### `resolve home directory: $HOME is not defined`

- The CLI now expands `~` by falling back to `/root` when `$HOME` is missing.
- If running as root in minimal environments, ensure `/root` exists or pass explicit paths (`--runtime-dir /var/lib/volant/run`).

### Kernel missing

- Ensure a Cloud Hypervisor compatible kernel with embedded initramfs exists at `/var/lib/volant/kernel/bzImage`.
- You can build the initramfs with `build/bake.sh` and then rebuild the kernel from https://github.com/cloud-hypervisor/linux using `CONFIG_INITRAMFS_SOURCE`.
- Alternatively, provide a URL to a prebuilt `bzImage` via the installer (`--kernel-url`) or place it manually at the expected path.

### systemd service launches CLI

- Generated unit now sets `ExecStart=/usr/local/bin/volantd`.
- If upgrading from older versions, remove `/etc/systemd/system/volantd.service` and rerun `sudo volar setup`.

## Networking issues

- Check bridge: `ip addr show vbr0`
- Restart iptables NAT: `sudo iptables -t nat -A POSTROUTING ...`
- Verify IP forwarding: `sysctl net.ipv4.ip_forward`

## Agent connectivity failures (`502 Bad Gateway`)

- VM must be running and have IP assigned.
- Ensure `cloud-hypervisor` process is active.
- Check agent logs via `volar browsers proxy` or `/ws/v1/vms/{name}/logs`.

## CLI problems

- Run with `--api` if server not on localhost.
- Logs appear in `~/.volant/logs/` and `/var/log/volant/server.log`.

## Installer issues

- Review `scripts/install.sh` outputs; run with `--dry-run` to inspect.
- Ensure GitHub releases accessible (corporate proxies may block downloads).

## Contact

Open GitHub issues with logs and reproduction steps.
