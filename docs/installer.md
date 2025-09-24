---
title: "Installation"
description: "Install Overhyped using the curl|bash bootstrapper and hype setup."
---

# Installation Guide

Overhyped ships a "zero-configuration" installer that bootstraps the host and launches the control plane in minutes.

## Prerequisites

- Linux host (tested on Ubuntu 24.04 LTS, Debian 12, Fedora 40).
- `curl`, `tar`, `sha256sum` (auto-installed if missing).
- `cloud-hypervisor`, `qemu-utils`, `bridge-utils`, `iptables` packages.
- `sudo` privileges (or run as root).

## Quick Install

```bash
curl -sSL https://github.com/ccheshirecat/overhyped/releases/latest/download/install.sh | bash
```

What the installer does:
1. Detects OS + architecture.
2. Installs prerequisites via the native package manager (prompted unless `--yes`).
3. Downloads the latest hype CLI/server/agent binaries from GitHub releases.
4. Verifies SHA-256 checksums.
5. Installs binaries to `/usr/local/bin/`.
6. Optionally runs `sudo hype setup`.

> Use `OVERHYPED_VERSION=v1.2.3` to pin a release, or download `install.sh` manually for review.

### Flags

```
install.sh [--version v1.2.3] [--force] [--skip-setup] [--yes]
```

- `--version` — install a specific tag instead of `latest`.
- `--force` — reinstall even if overhyped is already present.
- `--skip-setup` — skip invoking `hype setup` at the end.
- `--yes/-y` — non-interactive mode (auto-confirm dependency installs).

### Verify Installation

```bash
hype --version
hyped --help
hype-agent --help
```

## Post-Install: `hype setup`

`hype setup` provisions networking, installs the systemd service, and configures env vars:

```
sudo hype setup \
  --kernel /usr/local/share/overhyped/vmlinux-x86_64 \
  --initramfs /usr/local/share/overhyped/overhyped-initramfs.cpio.gz
```

Key responsibilities:

1. Create Linux bridge `hypebr0` (`ip link add` + `ip addr replace` + `ip link set up`).
2. Enable IP forwarding (`/proc/sys/net/ipv4/ip_forward`).
3. Configure idempotent NAT rules (`iptables -t nat POSTROUTING MASQUERADE`).
4. Ensure `~/.overhyped/run` and `~/.overhyped/logs` directories exist.
5. Write `/etc/systemd/system/hyped.service` with:
   - `ExecStart=/usr/local/bin/hyped`
   - `Environment` variables for kernel/initramfs/bridge/runtime dirs.
   - Log redirection to `/var/log/overhyped/server.log`.
6. (Optional) `systemctl daemon-reload && systemctl enable --now hyped`.

> Generate artifacts with `make build-images` if you prefer local kernel/initramfs; pass their paths via `--kernel` / `--initramfs` or environment variables (`OVERHYPED_KERNEL`, `OVERHYPED_INITRAMFS`).

## Manual Uninstall

```bash
sudo systemctl disable --now hyped
sudo rm -f /etc/systemd/system/hyped.service
sudo ip link delete hypebr0
sudo iptables -t nat -D POSTROUTING -s 192.168.127.0/24 ! -o hypebr0 -j MASQUERADE
rm -rf ~/.overhyped
sudo rm -f /usr/local/bin/hype /usr/local/bin/hyped /usr/local/bin/hype-agent
```

Adjust the iptables/bridge commands if you changed defaults.

## Troubleshooting

- **`/dev/tty` errors in systemd**: Ensure the unit’s `ExecStart` points to `hyped`, not the CLI.
- **Missing kernel/initramfs**: Verify `make build-images` output; supply correct paths to `hype setup`.
- **Networking issues**: Check `ip addr show hypebr0`, `ip route`, and `iptables -t nat -L -n` to confirm bridge and MASQUERADE rules.
- **Logs**: `/var/log/overhyped/server.log`, `journalctl -u hyped`, and event stream (`hype events tail`).

Need help? Open an issue on GitHub.
