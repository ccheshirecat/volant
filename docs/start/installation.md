---
title: "Installation"
description: "Install Volant using the curl|bash bootstrapper and volar setup."
---

# Installation Guide

Volant ships a "zero-configuration" installer that bootstraps the host and launches the control plane in minutes. The kernel initramfs is baked into the kernel bzImage used by Cloud Hypervisor.

## Prerequisites

- Linux host (tested on Ubuntu 24.04 LTS, Debian 12, Fedora 40).
- `curl`, `tar`, `sha256sum` (auto-installed if missing).
- `cloud-hypervisor`, `qemu-utils`, `bridge-utils`, `iptables` packages.
- `root privileges`. The volantd process runs as the root user by default. It might be possible to run as an unprivileged user, but we don't recommend it.

## Quick Install

```bash
curl -sSL https://github.com/volantvm/volant/releases/latest/download/install.sh | bash
```

What the installer does:
1. Detects OS + architecture.
2. Installs prerequisites via the native package manager (prompted unless `--yes`).
3. Downloads the latest `volar` CLI / `volantd` daemon / `kestrel` binaries from GitHub releases.
4. Provisions kernels at `/var/lib/volant/kernel/{bzImage,vmlinux}` as available. By default it fetches `bzImage` from repo `kernels/<arch>/bzImage` if available or use `--kernel-url`.
5. Verifies SHA-256 checksums.
6. Installs binaries to `/usr/local/bin/`.
7. Runs `sudo volar setup` (unless `--skip-setup`) which writes a systemd unit with `WorkingDirectory=/var/lib/volant` and kernel envs `VOLANT_KERNEL_BZIMAGE` and `VOLANT_KERNEL_VMLINUX` (when present), then enables and starts it.

> Use `VOLANT_VERSION=v2.0.0` to pin a release, or download `install.sh` manually for review.

### Flags

```
install.sh [--version v2.0.0] [--force] [--skip-setup] [--kernel-url <url>] [--yes]
```

- `--version` — install a specific tag instead of `latest`.
- `--force` — reinstall even if Volant is already present.
- `--skip-setup` — skip invoking `volar setup` at the end.
- `--kernel-url` — explicit URL to a `bzImage` to install at `/var/lib/volant/kernel/bzImage`.
- `--yes/-y` — non-interactive mode (auto-confirm dependency installs).

### Verify Installation

```bash
volar --version
volantd --help
kestrel --help
```

## Post-Install: `volar setup`

`volar setup` provisions networking, installs the systemd service, and configures env vars:

```
sudo volar setup \
  --work-dir /var/lib/volant \
  --bzimage /var/lib/volant/kernel/bzImage \
  --vmlinux /var/lib/volant/kernel/vmlinux
```

Key responsibilities:

1. Create Linux bridge `vbr0` (`ip link add` + `ip addr replace` + `ip link set up`).
2. Enable IP forwarding (`/proc/sys/net/ipv4/ip_forward`).
3. Configure idempotent NAT rules (`iptables -t nat POSTROUTING MASQUERADE`).
4. Ensure `~/.volant/run` and `~/.volant/logs` directories exist.
5. Write `/etc/systemd/system/volantd.service` with:
   - `ExecStart=/usr/local/bin/volantd`
   - `WorkingDirectory=/var/lib/volant`
   - `Environment` variables for bridge/runtime dirs and kernels (`VOLANT_KERNEL_BZIMAGE=/var/lib/volant/kernel/bzImage`, `VOLANT_KERNEL_VMLINUX=/var/lib/volant/kernel/vmlinux` if present)
   - Log redirection to `~/.volant/logs/volantd.log`.
6. `systemctl daemon-reload && systemctl enable --now volantd`.

> Dual-kernel: For rootfs mode, the initramfs is baked into the bzImage. For initramfs mode, provide a `vmlinux` kernel and pass a plugin `initramfs`. To build a custom initramfs, use `build/bake.sh` (supports `--copy src:dest` to inject files) and either: (a) rebuild the Cloud Hypervisor kernel with `CONFIG_INITRAMFS_SOURCE` to produce a bzImage, or (b) distribute the initramfs alongside `vmlinux`.

## Manual Uninstall

```bash
sudo systemctl disable --now volantd
sudo rm -f /etc/systemd/system/volantd.service
sudo ip link delete vbr0
sudo iptables -t nat -D POSTROUTING -s 192.168.127.0/24 ! -o vbr0 -j MASQUERADE || true
rm -rf ~/.volant
sudo rm -f /usr/local/bin/volar /usr/local/bin/volantd /usr/local/bin/kestrel
```

Adjust the iptables/bridge commands if you changed defaults.

## Troubleshooting

- **`/dev/tty` errors in systemd**: Ensure the unit’s `ExecStart` points to `volantd`, not the CLI.
- **Missing kernel/initramfs**: Verify `make build-browser-artifacts` output; supply correct paths to `volar setup`.
- **Networking issues**: Check `ip addr show vbr0`, `ip route`, and `iptables -t nat -L -n` to confirm bridge and MASQUERADE rules.
- **Logs**: `/var/log/volant/volantd.log`, `journalctl -u volantd`, and event stream (`volar vms watch`).

Need help? Open an issue on GitHub.
