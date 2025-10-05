# Installation

This guide reflects the actual installer and setup logic in this repository.

References:
- scripts/install.sh
- internal/setup and the CLI command `volar setup`

## Quick install

```bash
# Official installer (branding shortcut)
curl -fsSL https://get.volantvm.com | bash
```

The endpoint points to the same script hosted in this repository:
- https://raw.githubusercontent.com/volantvm/volant/main/scripts/install.sh

The script will:
- Download Volant binaries (volar, volantd, kestrel) from GitHub Releases
- Download kernel artifacts: bzImage (from Volant release) and vmlinux (from Cloud Hypervisor)
- Install to /usr/local/bin and /var/lib/volant/kernel
- Optionally run `sudo volar setup` (recommended)

## Setup (networking + systemd)

`volar setup` configures host networking and writes a systemd unit for volantd.

Defaults (from code):
- Bridge: vbr0
- Subnet: 192.168.127.0/24
- Host IP in bridge: 192.168.127.1/24
- Work dir: /var/lib/volant
- Kernel paths:
  - bzImage: /var/lib/volant/kernel/bzImage
  - vmlinux: /var/lib/volant/kernel/vmlinux

What it does:
- Creates bridge vbr0, assigns host IP, brings it up
- Enables IPv4 forwarding
- Adds iptables MASQUERADE for the managed subnet
- Writes systemd unit for volantd and enables it

Non-interactive example:
```bash
sudo volar setup \
  --work-dir /var/lib/volant \
  --bridge vbr0 \
  --subnet 192.168.127.0/24 \
  --host-ip 192.168.127.1 \
  --bzimage /var/lib/volant/kernel/bzImage \
  --vmlinux /var/lib/volant/kernel/vmlinux
```

Environment overrides:
- VOLANT_BRIDGE, VOLANT_SUBNET, VOLANT_RUNTIME_DIR, VOLANT_LOG_DIR,
  VOLANT_KERNEL_BZIMAGE, VOLANT_KERNEL_VMLINUX, VOLANT_WORK_DIR

## Requirements

- Linux host with:
  - cloud-hypervisor in PATH
  - qemu-img (from qemu-utils)
  - bridge-utils (brctl) and iptables
  - sha256sum (coreutils)
- Sudo privileges (or run installer as root)
- x86_64 architecture

The installer can auto-install missing packages for Ubuntu/Debian, Fedora, RHEL/CentOS, and Arch-based distros.

## Verify installation

```bash
volar --version
volantd --version
kestrel --version

# Control plane status
curl -s http://127.0.0.1:8080/healthz
```

If you skipped setup during install, run it later:
```bash
sudo volar setup --work-dir /var/lib/volant
```

## Uninstall / cleanup

- Stop volantd: `sudo systemctl disable --now volantd`
- Remove bridge and iptables rules manually if you customized them
- Remove work directory `/var/lib/volant` if desired

## Troubleshooting

- If `cloud-hypervisor` not found: install package from your distro or upstream
- Ensure `/proc/sys/net/ipv4/ip_forward` is `1`
- Check service logs: `journalctl -u volantd -f`
- Networking conflicts: ensure `vbr0` bridge does not collide with existing networks
