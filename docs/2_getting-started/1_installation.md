# Installation

Get Volant running in under 60 seconds.

---

## Prerequisites

### System Requirements

- **Linux** (kernel 4.20+ with KVM support)
- **x86_64 or aarch64** processor with virtualization enabled
- **2GB RAM minimum** (4GB+ recommended for multiple VMs)
- **10GB disk space** (for binaries, kernels, and VM artifacts)
- **Root access** (for network configuration and VM management)

### Check KVM Support

Verify your system supports hardware virtualization:

```bash
# Check for virtualization extensions
egrep -c '(vmx|svm)' /proc/cpuinfo
# Should return > 0

# Check if KVM modules are loaded
lsmod | grep kvm
# Should show kvm_intel or kvm_amd

# Verify KVM device exists
ls -l /dev/kvm
# Should show: crw-rw-rw- 1 root kvm ...
```

If KVM is not enabled:

```bash
# Load KVM modules
sudo modprobe kvm
sudo modprobe kvm_intel  # Or kvm_amd for AMD processors

# Make it persistent
echo "kvm" | sudo tee -a /etc/modules
echo "kvm_intel" | sudo tee -a /etc/modules  # Or kvm_amd
```

---

## Quick Install (Recommended)

Use the official install script to download and configure everything automatically:

```bash
curl -sSL https://install.volantvm.com | bash
```

This script will:
1. Detect your architecture (x86_64 or aarch64)
2. Download the latest release binaries (`volantd`, `volar`, `kestrel`)
3. Install them to `/usr/local/bin`
4. Download the dual kernels (`bzImage`, `vmlinux`)
5. Install kernels to `/var/lib/volant/kernel/`
6. Download the initramfs bootloader (`volant-initramfs.cpio.gz`)
7. Install it to `/var/lib/volant/`
8. Set appropriate permissions

### Verify Installation

```bash
# Check binary versions
volantd --version
volar --version

# Check kernel files
ls -lh /var/lib/volant/kernel/
# Should show bzImage and vmlinux

# Check initramfs
ls -lh /var/lib/volant/volant-initramfs.cpio.gz
```

---

## Manual Installation

If you prefer manual installation or the script doesn't work for your environment:

### Step 1: Download Binaries

```bash
# Set your architecture
ARCH=$(uname -m)  # x86_64 or aarch64
VERSION="v0.1.0"  # Or latest from GitHub releases

# Create directory
sudo mkdir -p /usr/local/bin

# Download binaries
curl -L "https://github.com/volantvm/volant/releases/download/${VERSION}/volantd-linux-${ARCH}" -o /tmp/volantd
curl -L "https://github.com/volantvm/volant/releases/download/${VERSION}/volar-linux-${ARCH}" -o /tmp/volar
curl -L "https://github.com/volantvm/volant/releases/download/${VERSION}/kestrel-linux-${ARCH}" -o /tmp/kestrel

# Install binaries
sudo mv /tmp/volantd /usr/local/bin/volantd
sudo mv /tmp/volar /usr/local/bin/volar
sudo mv /tmp/kestrel /usr/local/bin/kestrel

# Make executable
sudo chmod +x /usr/local/bin/{volantd,volar,kestrel}
```

### Step 2: Download Kernels and Initramfs

```bash
# Create directory
sudo mkdir -p /var/lib/volant/kernel

# Download kernels
curl -L "https://github.com/volantvm/volant/releases/download/${VERSION}/bzImage" -o /tmp/bzImage
curl -L "https://github.com/volantvm/volant/releases/download/${VERSION}/vmlinux" -o /tmp/vmlinux

# Download initramfs bootloader
curl -L "https://github.com/volantvm/volant/releases/download/${VERSION}/volant-initramfs.cpio.gz" -o /tmp/volant-initramfs.cpio.gz

# Install
sudo mv /tmp/bzImage /var/lib/volant/kernel/bzImage
sudo mv /tmp/vmlinux /var/lib/volant/kernel/vmlinux
sudo mv /tmp/volant-initramfs.cpio.gz /var/lib/volant/volant-initramfs.cpio.gz

# Set permissions
sudo chmod 644 /var/lib/volant/kernel/*
sudo chmod 644 /var/lib/volant/volant-initramfs.cpio.gz
```

### Step 3: Verify

```bash
volantd --version
volar --version
ls -lh /var/lib/volant/kernel/
```

---

## Host Configuration

Configure networking and system services:

```bash
# Configure the host (one-time setup)
sudo volar setup

# This will:
# 1. Create the vbr0 bridge network
# 2. Configure NAT for internet access
# 3. Set up IP forwarding
# 4. Create systemd service for volantd
# 5. Initialize the SQLite database
```

### What `volar setup` Does

The setup command performs the following:

#### 1. Network Bridge Creation

Creates a Linux bridge (`vbr0`) with subnet `192.168.127.0/24`:

```bash
sudo ip link add vbr0 type bridge
sudo ip addr add 192.168.127.1/24 dev vbr0
sudo ip link set vbr0 up
```

#### 2. NAT Configuration

Enables internet access for VMs via NAT:

```bash
sudo iptables -t nat -A POSTROUTING -s 192.168.127.0/24 -j MASQUERADE
sudo iptables -A FORWARD -i vbr0 -j ACCEPT
sudo iptables -A FORWARD -o vbr0 -j ACCEPT
```

#### 3. IP Forwarding

Enables kernel IP forwarding:

```bash
sudo sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
```

#### 4. Systemd Service

Creates `/etc/systemd/system/volantd.service`:

```ini
[Unit]
Description=Volant Control Plane
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/volantd
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enables and starts the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable volantd
sudo systemctl start volantd
```

#### 5. Database Initialization

Creates the SQLite database at `/var/lib/volant/volant.db` with all required tables.

---

## Verify Setup

### Check Network Bridge

```bash
ip addr show vbr0
# Should show: inet 192.168.127.1/24

ip link show vbr0
# Should show: state UP
```

### Check volantd Service

```bash
sudo systemctl status volantd
# Should show: active (running)

# Check logs
sudo journalctl -u volantd -f
```

### Check Database

```bash
sudo sqlite3 /var/lib/volant/volant.db ".tables"
# Should show: plugins, vms, ip_leases, deployments, events
```

### Test CLI Connectivity

```bash
volar plugins list
# Should return empty list (no plugins installed yet)

volar vms list
# Should return empty list (no VMs created yet)
```

---

## Install Cloud Hypervisor (Optional)

Volant expects `cloud-hypervisor` to be available in your PATH. If you don't have it installed:

### From Package Manager (Ubuntu/Debian)

```bash
# Add Cloud Hypervisor PPA (if available)
# Or download from GitHub releases
```

### From GitHub Releases

```bash
VERSION="v38.0"  # Or latest
ARCH=$(uname -m)

curl -L "https://github.com/cloud-hypervisor/cloud-hypervisor/releases/download/${VERSION}/cloud-hypervisor-${ARCH}" -o /tmp/cloud-hypervisor

sudo mv /tmp/cloud-hypervisor /usr/local/bin/cloud-hypervisor
sudo chmod +x /usr/local/bin/cloud-hypervisor

# Verify
cloud-hypervisor --version
```

---

## Configuration

Volant looks for configuration in these locations (in order):

1. Command-line flags
2. Environment variables
3. `/etc/volant/config.yaml`
4. `~/.config/volant/config.yaml`
5. Built-in defaults

### Default Configuration

```yaml
# /etc/volant/config.yaml
server:
  listen_addr: "127.0.0.1:8080"
  data_dir: "/var/lib/volant"

database:
  path: "/var/lib/volant/volant.db"

networking:
  bridge_name: "vbr0"
  subnet: "192.168.127.0/24"
  gateway: "192.168.127.1"
  dhcp_start: "192.168.127.100"
  dhcp_end: "192.168.127.254"

kernel:
  bzimage_path: "/var/lib/volant/kernel/bzImage"
  vmlinux_path: "/var/lib/volant/kernel/vmlinux"
  initramfs_path: "/var/lib/volant/volant-initramfs.cpio.gz"

logging:
  level: "info"  # debug, info, warn, error
  format: "text"  # text or json
```

### Environment Variables

Override configuration with environment variables:

```bash
export VOLANT_LISTEN_ADDR="0.0.0.0:8080"
export VOLANT_DATA_DIR="/custom/path"
export VOLANT_LOG_LEVEL="debug"
```

---

## Troubleshooting

### KVM Permission Denied

If you get "permission denied" errors accessing `/dev/kvm`:

```bash
# Add your user to the kvm group
sudo usermod -a -G kvm $USER

# Log out and back in, or:
newgrp kvm
```

### Bridge Network Not Working

Check if the bridge exists and has the correct IP:

```bash
ip addr show vbr0
```

Recreate if needed:

```bash
sudo ip link del vbr0
sudo volar setup
```

### volantd Won't Start

Check the logs:

```bash
sudo journalctl -u volantd -n 50
```

Common issues:
- Port 8080 already in use (change `listen_addr` in config)
- Database permissions (`sudo chown volant:volant /var/lib/volant/volant.db`)
- Missing kernel files (check `/var/lib/volant/kernel/`)

### Cloud Hypervisor Not Found

Ensure `cloud-hypervisor` is in your PATH:

```bash
which cloud-hypervisor
```

If not found, install it (see above).

---

## Uninstallation

To completely remove Volant:

```bash
# Stop the service
sudo systemctl stop volantd
sudo systemctl disable volantd
sudo rm /etc/systemd/system/volantd.service
sudo systemctl daemon-reload

# Remove binaries
sudo rm /usr/local/bin/{volantd,volar,kestrel}

# Remove data (WARNING: This deletes all VMs and plugins!)
sudo rm -rf /var/lib/volant

# Remove bridge network
sudo ip link del vbr0

# Remove NAT rules (careful if you have other rules)
sudo iptables -t nat -D POSTROUTING -s 192.168.127.0/24 -j MASQUERADE
sudo iptables -D FORWARD -i vbr0 -j ACCEPT
sudo iptables -D FORWARD -o vbr0 -j ACCEPT

# Remove IP forwarding (only if you don't need it for other purposes)
sudo sysctl -w net.ipv4.ip_forward=0
sudo sed -i '/net.ipv4.ip_forward=1/d' /etc/sysctl.conf
```

---

## Next Steps

Now that Volant is installed, you're ready to create your first microVM:

- **[Quick Start: Rootfs](2_quick-start-rootfs.md)** — Run an OCI image (NGINX demo)
- **[Quick Start: Initramfs](3_quick-start-initramfs.md)** — Deploy a custom appliance (Caddy demo)
- **[CLI Reference](../3_guides/1_cli-reference.md)** — Complete command documentation
- **[Plugin Guide](../3_guides/2_plugins.md)** — Install and manage plugins

---

*Installation complete. Let's build something.*
.*
