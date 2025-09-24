---
title: "Installation"
description: "Install Viper using the curl|bash bootstrapper and viper setup."
---

# Installation Guide

Viper ships a "zero-configuration" installer that bootstraps the host and launches the control plane in minutes.

## Prerequisites

- Linux host (tested on Ubuntu 24.04 LTS, Debian 12, Fedora 40).
- `curl`, `tar`, `sha256sum` (auto-installed if missing).
- `cloud-hypervisor`, `qemu-utils`, `bridge-utils`, `iptables` packages.
- `sudo` privileges (or run as root).

## Quick Install

```bash
curl -sSL https://github.com/ccheshirecat/viper/releases/latest/download/install.sh | bash
```

What the installer does:
1. Detects OS + architecture.
2. Installs prerequisites via the native package manager (prompted unless `--yes`).
3. Downloads the latest Viper CLI/server/agent binaries from GitHub releases.
4. Verifies SHA-256 checksums.
5. Installs binaries to `/usr/local/bin/`.
6. Optionally runs `sudo viper setup`.

> Use `VIPER_VERSION=v1.2.3` to pin a release, or download `install.sh` manually for review.

### Flags

```
install.sh [--version v1.2.3] [--force] [--skip-setup] [--yes]
```

- `--version` — install a specific tag instead of `latest`.
- `--force` — reinstall even if Viper is already present.
- `--skip-setup` — skip invoking `viper setup` at the end.
- `--yes/-y` — non-interactive mode (auto-confirm dependency installs).

### Verify Installation

```bash
viper --version
viper-server --help
viper-agent --help
```

## Post-Install: `viper setup`

`viper setup` provisions networking, installs the systemd service, and configures env vars:

```
sudo viper setup \
  --kernel /usr/local/share/viper/vmlinux-x86_64 \
  --initramfs /usr/local/share/viper/viper-initramfs.cpio.gz
```

Key responsibilities:

1. Create Linux bridge `viperbr0` (`ip link add` + `ip addr replace` + `ip link set up`).
2. Enable IP forwarding (`/proc/sys/net/ipv4/ip_forward`).
3. Configure idempotent NAT rules (`iptables -t nat POSTROUTING MASQUERADE`).
4. Ensure `~/.viper/run` and `~/.viper/logs` directories exist.
5. Write `/etc/systemd/system/viper-server.service` with:
   - `ExecStart=/usr/local/bin/viper-server`
   - `Environment` variables for kernel/initramfs/bridge/runtime dirs.
   - Log redirection to `/var/log/viper/server.log`.
6. (Optional) `systemctl daemon-reload && systemctl enable --now viper-server`.

> Generate artifacts with `make build-images` if you prefer local kernel/initramfs; pass their paths via `--kernel` / `--initramfs` or environment variables (`VIPER_KERNEL`, `VIPER_INITRAMFS`).

## Manual Uninstall

```bash
sudo systemctl disable --now viper-server
sudo rm -f /etc/systemd/system/viper-server.service
sudo ip link delete viperbr0
sudo iptables -t nat -D POSTROUTING -s 192.168.127.0/24 ! -o viperbr0 -j MASQUERADE
rm -rf ~/.viper
sudo rm -f /usr/local/bin/viper /usr/local/bin/viper-server /usr/local/bin/viper-agent
```

Adjust the iptables/bridge commands if you changed defaults.

## Troubleshooting

- **`/dev/tty` errors in systemd**: Ensure the unit’s `ExecStart` points to `viper-server`, not the CLI.
- **Missing kernel/initramfs**: Verify `make build-images` output; supply correct paths to `viper setup`.
- **Networking issues**: Check `ip addr show viperbr0`, `ip route`, and `iptables -t nat -L -n` to confirm bridge and MASQUERADE rules.
- **Logs**: `/var/log/viper/server.log`, `journalctl -u viper-server`, and event stream (`viper events tail`).

Need help? Open an issue on GitHub.
