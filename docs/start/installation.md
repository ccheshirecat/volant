---
title: "Installation"
description: "Install Volant using the curl|bash bootstrapper and volar setup."
---

# Installation Guide

Volant ships a "zero-configuration" installer that bootstraps the host and launches the control plane in minutes.

## Prerequisites

- Linux host (tested on Ubuntu 24.04 LTS, Debian 12, Fedora 40).
- `curl`, `tar`, `sha256sum` (auto-installed if missing).
- `cloud-hypervisor`, `qemu-utils`, `bridge-utils`, `iptables` packages.
- `root privileges`. The volantd process runs as the root user by default. It might be possible to run as an unprivileged user, but we don't recommend it.

## Quick Install

```bash
curl -sSL https://github.com/ccheshirecat/volant/releases/latest/download/install.sh | bash
```

What the installer does:
1. Detects OS + architecture.
2. Installs prerequisites via the native package manager (prompted unless `--yes`).
3. Downloads the latest `volar` CLI / `volantd` daemon / `volary` binaries from GitHub releases.
4. Fetches the default runtime bundle (kernel, initramfs, manifest) and stores it under `/usr/local/share/volant` for quick-start scenarios.
5. Verifies SHA-256 checksums.
6. Installs binaries to `/usr/local/bin/`.
7. Optionally runs `sudo volar setup`.

> Use `VOLANT_VERSION=v2.0.0` to pin a release, or download `install.sh` manually for review.

### Flags

```
install.sh [--version v2.0.0] [--force] [--skip-setup] [--yes]
```

- `--version` — install a specific tag instead of `latest`.
- `--force` — reinstall even if Volant is already present.
- `--skip-setup` — skip invoking `volar setup` at the end.
- `--yes/-y` — non-interactive mode (auto-confirm dependency installs).

### Verify Installation

```bash
volar --version
volantd --help
volary --help
```

## Post-Install: `volar setup`

`volar setup` provisions networking, installs the systemd service, and configures env vars. It also seeds the runtime directory with any manifests staged by the installer:

```
sudo volar setup \
  --kernel /usr/local/share/volant/vmlinux-x86_64 \
  --initramfs /usr/local/share/volant/volant-initramfs.cpio.gz
```

Key responsibilities:

1. Create Linux bridge `vbr0` (`ip link add` + `ip addr replace` + `ip link set up`).
2. Enable IP forwarding (`/proc/sys/net/ipv4/ip_forward`).
3. Configure idempotent NAT rules (`iptables -t nat POSTROUTING MASQUERADE`).
4. Ensure `~/.volant/run` and `~/.volant/logs` directories exist.
5. Write `/etc/systemd/system/volantd.service` with:
   - `ExecStart=/usr/local/bin/volantd`
   - `Environment` variables for kernel/initramfs/bridge/runtime dirs.
   - Log redirection to `/var/log/volant/volantd.log`.
6. (Optional) `systemctl daemon-reload && systemctl enable --now volantd`.

> Generate artifacts with `make build-browser-artifacts` if you prefer local kernel/initramfs; pass their paths via `--kernel` / `--initramfs` or environment variables (`VOLANT_KERNEL`, `VOLANT_INITRAMFS`).

## Manual Uninstall

```bash
sudo systemctl disable --now volantd
sudo rm -f /etc/systemd/system/volantd.service
sudo ip link delete vbr0
sudo iptables -t nat -D POSTROUTING -s 192.168.127.0/24 ! -o vbr0 -j MASQUERADE
rm -rf ~/.volant
sudo rm -f /usr/local/bin/volar /usr/local/bin/volantd /usr/local/bin/volary
```

Adjust the iptables/bridge commands if you changed defaults.

## Troubleshooting

- **`/dev/tty` errors in systemd**: Ensure the unit’s `ExecStart` points to `volantd`, not the CLI.
- **Missing kernel/initramfs**: Verify `make build-browser-artifacts` output; supply correct paths to `volar setup`.
- **Networking issues**: Check `ip addr show vbr0`, `ip route`, and `iptables -t nat -L -n` to confirm bridge and MASQUERADE rules.
- **Logs**: `/var/log/volant/volantd.log`, `journalctl -u volantd`, and event stream (`volar vms watch`).

Need help? Open an issue on GitHub.
