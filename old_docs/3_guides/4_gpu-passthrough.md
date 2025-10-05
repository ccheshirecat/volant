# GPU Passthrough for AI/ML Workloads

Volant supports **VFIO GPU passthrough** for AI/ML workloads, enabling you to run inference, training, and compute-intensive tasks on bare-metal GPU performance inside microVMs.

## Overview

GPU passthrough uses Linux VFIO (Virtual Function I/O) to map physical GPU devices directly into microVMs. This provides:

- **Native GPU performance** - no virtualization overhead
- **Full driver support** - NVIDIA CUDA, AMD ROCm, Intel oneAPI
- **Direct hardware access** - complete GPU control for inference/training
- **Isolation** - each VM gets exclusive GPU access

## Prerequisites

### 1. Hardware Requirements

- **IOMMU-capable CPU/Motherboard**
  - Intel: VT-d enabled in BIOS
  - AMD: AMD-Vi enabled in BIOS
- **Dedicated GPU** for passthrough (not the host display GPU)
- **PCI device in separate IOMMU group** (check with `lspci` and `/sys/kernel/iommu_groups/`)

### 2. Host System Setup

#### Enable IOMMU in GRUB

Edit `/etc/default/grub`:

```bash
# Intel
GRUB_CMDLINE_LINUX="intel_iommu=on iommu=pt"

# AMD
GRUB_CMDLINE_LINUX="amd_iommu=on iommu=pt"
```

Update GRUB and reboot:

```bash
sudo update-grub
sudo reboot
```

#### Verify IOMMU is Active

```bash
dmesg | grep -i iommu
# Should see: "IOMMU enabled" or "AMD-Vi: AMD IOMMUv2 loaded and initialized"
```

#### Find Your GPU PCI Address

```bash
lspci | grep -i vga
# Example output:
# 0000:01:00.0 VGA compatible controller: NVIDIA Corporation GA102 [GeForce RTX 3090]
```

#### Unbind GPU from Host Driver

```bash
# Identify current driver
lspci -k -s 0000:01:00.0

# Unbind from host driver (e.g., nvidia, nouveau, radeon)
echo "0000:01:00.0" | sudo tee /sys/bus/pci/drivers/nvidia/unbind

# Bind to vfio-pci
echo "10de 2204" | sudo tee /sys/bus/pci/drivers/vfio-pci/new_id  # vendor:device IDs
echo "0000:01:00.0" | sudo tee /sys/bus/pci/drivers/vfio-pci/bind
```

**Note**: Volant will automatically bind devices to `vfio-pci` when creating VMs, but manual setup may be needed if the GPU is in use by the host.

## Creating GPU-Enabled VMs

### Method 1: CLI Flags

Pass GPU devices directly via command-line:

```bash
volar vms create llm-worker \
  --plugin pytorch \
  --cpu 8 \
  --memory 16384 \
  --device 0000:01:00.0 \
  --device-allowlist "10de:*"
```

**Flags:**
- `--device <pci-address>`: PCI address of the GPU to pass through
- `--device-allowlist <pattern>`: Security allowlist (e.g., `10de:*` for all NVIDIA, `1002:*` for AMD)

### Method 2: Plugin Manifest

Define GPU passthrough in the plugin manifest:

```json
{
  "schema_version": "1.0",
  "name": "pytorch-gpu",
  "version": "1.0.0",
  "runtime": "pytorch-gpu",
  "initramfs": {
    "url": "https://releases.volantvm.com/plugins/pytorch-gpu-v1.0.0.cpio.gz",
    "checksum": "sha256:abc123..."
  },
  "resources": {
    "cpu_cores": 8,
    "memory_mb": 16384
  },
  "devices": {
    "pci_passthrough": ["0000:01:00.0"],
    "allowlist": ["10de:*"]
  },
  "workload": {
    "type": "http",
    "base_url": "http://127.0.0.1:8000",
    "entrypoint": ["/usr/local/bin/pytorch-server"]
  },
  "health_check": {
    "endpoint": "/health",
    "timeout_ms": 30000
  }
}
```

Install and launch:

```bash
volar plugins install https://example.com/pytorch-gpu-manifest.json
volar vms create inference-1 --plugin pytorch-gpu
```

### Method 3: Deployment Config

For Kubernetes-style deployments with GPU replicas:

```json
{
  "plugin": "pytorch-gpu",
  "runtime": "pytorch-gpu",
  "resources": {
    "cpu_cores": 8,
    "memory_mb": 16384
  },
  "manifest": {
    "devices": {
      "pci_passthrough": ["0000:01:00.0", "0000:02:00.0"],
      "allowlist": ["10de:*"]
    }
  }
}
```

Create deployment:

```bash
volar deployments create ai-cluster --config deployment.json --replicas 2
```

## Guest VM Setup

### Installing GPU Drivers

The guest VM needs GPU drivers to use the passed-through device.

#### NVIDIA CUDA (Initramfs)

Build a static initramfs plugin with NVIDIA drivers:

```dockerfile
FROM nvidia/cuda:12.3.0-devel-ubuntu22.04 AS builder

# Install static compilation tools
RUN apt-get update && apt-get install -y \
    build-essential \
    musl-tools \
    upx

# Build your application
COPY . /app
WORKDIR /app
RUN make static

# Create initramfs with drivers
FROM scratch
COPY --from=builder /app/bin/pytorch-server /usr/local/bin/
COPY --from=builder /usr/local/cuda-12.3 /usr/local/cuda
ENV LD_LIBRARY_PATH=/usr/local/cuda/lib64
CMD ["/usr/local/bin/pytorch-server"]
```

Package with Fledge:

```toml
[fledge]
name = "pytorch-gpu"
version = "1.0.0"

[image]
dockerfile = "Dockerfile"
context = "."

[initramfs]
busybox_version = "1.36.1"
init_script = "init.sh"
modules = []
```

#### NVIDIA CUDA (OCI Rootfs)

Use official NVIDIA CUDA images:

```json
{
  "image": "nvidia/cuda:12.3.0-runtime-ubuntu22.04",
  "rootfs": {
    "url": "https://releases.volantvm.com/rootfs/nvidia-cuda-12.3.0.qcow2",
    "format": "qcow2"
  }
}
```

## Verification

### Check GPU Inside VM

```bash
# Attach to VM console
volar vms console llm-worker

# Inside VM - check PCI devices
lspci | grep -i nvidia

# Verify NVIDIA driver
nvidia-smi
```

Expected output:

```
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 535.129.03   Driver Version: 535.129.03   CUDA Version: 12.3    |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|                               |                      |               MIG M. |
|===============================+======================+======================|
|   0  NVIDIA GeForce ... On   | 0000:01:00.0     Off |                  N/A |
| 30%   45C    P8    25W / 350W |      0MiB / 24576MiB |      0%      Default |
+-------------------------------+----------------------+----------------------+
```

### Test CUDA

```python
import torch
print(f"CUDA available: {torch.cuda.is_available()}")
print(f"GPU count: {torch.cuda.device_count()}")
print(f"GPU name: {torch.cuda.get_device_name(0)}")
```

## Security: Device Allowlists

Use allowlists to restrict which devices can be passed through:

```json
{
  "devices": {
    "pci_passthrough": ["0000:01:00.0"],
    "allowlist": [
      "10de:*",        // All NVIDIA devices
      "1002:*",        // All AMD devices
      "8086:56a0"      // Specific Intel device
    ]
  }
}
```

**Vendor IDs:**
- NVIDIA: `10de`
- AMD: `1002`
- Intel: `8086`

Volant will validate devices at VM creation and reject any not matching the allowlist.

## Common Patterns

### LLM Inference

```bash
# Single GPU for model serving
volar vms create llama-serve \
  --plugin llama-cpp \
  --cpu 8 \
  --memory 32768 \
  --device 0000:01:00.0
```

### Distributed Training

```json
{
  "plugin": "pytorch-ddp",
  "devices": {
    "pci_passthrough": [
      "0000:01:00.0",
      "0000:02:00.0", 
      "0000:03:00.0",
      "0000:04:00.0"
    ]
  }
}
```

### Multi-Tenant AI Platform

```bash
# Tenant 1: GPU 0
volar vms create tenant1-gpu --plugin pytorch --device 0000:01:00.0

# Tenant 2: GPU 1
volar vms create tenant2-gpu --plugin pytorch --device 0000:02:00.0

# Tenant 3: GPU 2
volar vms create tenant3-gpu --plugin pytorch --device 0000:03:00.0
```

## Troubleshooting

### Device Not Found

```
Error: PCI device not found: 0000:01:00.0
```

**Solution**: Verify PCI address with `lspci` and check the device exists in `/sys/bus/pci/devices/`.

### Device Not Bound to vfio-pci

```
Error: device not bound to vfio-pci driver
```

**Solution**: Manually bind the device:

```bash
echo "0000:01:00.0" | sudo tee /sys/bus/pci/drivers/<current-driver>/unbind
echo "10de 2204" | sudo tee /sys/bus/pci/drivers/vfio-pci/new_id
echo "0000:01:00.0" | sudo tee /sys/bus/pci/drivers/vfio-pci/bind
```

### IOMMU Not Enabled

```
Error: device has no IOMMU group (IOMMU may not be enabled)
```

**Solution**: Enable IOMMU in BIOS and kernel cmdline (see Prerequisites).

### Device in Use by Host

```
Error: failed to bind device: device or resource busy
```

**Solution**: The host is using the GPU. Unbind from the host driver first, or use a dedicated GPU for passthrough.

### IOMMU Group Contains Multiple Devices

```
IOMMU group 1: 0000:01:00.0 0000:01:00.1
```

**Solution**: Pass through **all devices in the IOMMU group**, or use an ACS override patch (advanced, potential security implications).

## Performance Considerations

- **PCI Express Generation**: Use PCIe 4.0/5.0 for maximum bandwidth
- **NUMA Affinity**: Pin VM CPUs to the same NUMA node as the GPU for optimal memory access
- **Huge Pages**: Enable huge pages on the host for better memory performance
- **CPU Pinning**: Use dedicated CPU cores for GPU-bound workloads

## Limitations

- **One VM per GPU**: Physical GPUs cannot be shared between VMs without SR-IOV (future feature)
- **No Live Migration**: VMs with GPU passthrough cannot be live-migrated
- **Host Cannot Use GPU**: Once passed through, the host loses access to the device
- **Driver Compatibility**: Guest kernel must match GPU driver requirements

## Next Steps

- [Plugin Development Guide](../4_plugin-development/1_overview.md) - Build GPU-enabled plugins
- [Manifest Schema Reference](../6_reference/1_manifest-schema.md) - Full device configuration options
- [Architecture Overview](../5_architecture/1_overview.md) - How VFIO integration works
