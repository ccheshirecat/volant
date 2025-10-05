# VFIO Device Management API Reference

This document describes the HTTP API endpoints for managing VFIO (Virtual Function I/O) device passthrough in Volant. These endpoints allow you to discover, validate, bind, and unbind PCI devices for GPU and hardware passthrough to microVMs.

## Base URL

All VFIO endpoints are under `/api/v1/vfio`.

## Prerequisites

Before using these endpoints, ensure:

1. **IOMMU is enabled** in your system BIOS and kernel
2. **Root/sudo access** is available (device binding requires elevated privileges)
3. **vfio-pci kernel module** is loaded
4. Devices are **not in use** by the host

## Endpoints

### Get Device Information

Get detailed information about a specific PCI device.

**Endpoint:** `POST /api/v1/vfio/devices/info`

**Request Body:**

```json
{
  "pci_address": "0000:01:00.0"
}
```

**Response (200 OK):**

```json
{
  "address": "0000:01:00.0",
  "vendor": "10de",
  "device": "2204",
  "driver": "vfio-pci",
  "iommu_group": "1",
  "numa_node": "0"
}
```

**Fields:**

- `address` - PCI address in domain:bus:device.function format
- `vendor` - Vendor ID (e.g., `10de` for NVIDIA, `1002` for AMD)
- `device` - Device ID
- `driver` - Current driver bound to the device
- `iommu_group` - IOMMU group number (devices in same group must be passed together)
- `numa_node` - NUMA node the device is attached to

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/info \
  -H "Content-Type: application/json" \
  -d '{"pci_address": "0000:01:00.0"}'
```

---

### Validate Devices

Validate PCI addresses and check against an optional allowlist.

**Endpoint:** `POST /api/v1/vfio/devices/validate`

**Request Body:**

```json
{
  "pci_addresses": ["0000:01:00.0", "0000:01:00.1"],
  "allowlist": ["10de:*", "1002:*"]
}
```

**Fields:**

- `pci_addresses` (required) - Array of PCI addresses to validate
- `allowlist` (optional) - Array of vendor:device patterns (e.g., `10de:*` for all NVIDIA devices)

**Response (200 OK):**

```json
{
  "valid": true,
  "message": "All devices are valid and available for passthrough"
}
```

**Response (Validation Failed):**

```json
{
  "valid": false,
  "message": "invalid PCI address format: 01:00.0 (expected format: 0000:01:00.0)",
  "errors": [
    "invalid PCI address format: 01:00.0 (expected format: 0000:01:00.0)"
  ]
}
```

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/validate \
  -H "Content-Type: application/json" \
  -d '{
    "pci_addresses": ["0000:01:00.0"],
    "allowlist": ["10de:*"]
  }'
```

---

### Check IOMMU Groups

Get IOMMU group information for specified devices.

**Endpoint:** `POST /api/v1/vfio/devices/iommu-groups`

**Request Body:**

```json
{
  "pci_addresses": ["0000:01:00.0", "0000:01:00.1"]
}
```

**Response (200 OK):**

```json
[
  {
    "id": "1",
    "devices": [
      "0000:01:00.0",
      "0000:01:00.1"
    ]
  }
]
```

**Fields:**

- `id` - IOMMU group number
- `devices` - Array of all PCI devices in this IOMMU group

> **Note:** All devices in the same IOMMU group must be passed through together for security isolation.

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/iommu-groups \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'
```

---

### Bind Devices to vfio-pci

Bind PCI devices to the vfio-pci driver for passthrough.

**Endpoint:** `POST /api/v1/vfio/devices/bind`

**Request Body:**

```json
{
  "pci_addresses": ["0000:01:00.0", "0000:01:00.1"]
}
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Devices successfully bound to vfio-pci driver",
  "bound_devices": ["0000:01:00.0", "0000:01:00.1"]
}
```

**Response (500 Internal Server Error):**

```json
{
  "success": false,
  "message": "device 0000:01:00.0 is in use by host"
}
```

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/bind \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'
```

> **Warning:** Binding a device will unbind it from its current driver. Ensure the device is not being used by the host system.

---

### Unbind Devices from vfio-pci

Unbind PCI devices from vfio-pci and restore the original driver.

**Endpoint:** `POST /api/v1/vfio/devices/unbind`

**Request Body:**

```json
{
  "pci_addresses": ["0000:01:00.0", "0000:01:00.1"]
}
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Devices successfully unbound from vfio-pci driver",
  "unbound_devices": ["0000:01:00.0", "0000:01:00.1"]
}
```

**Response (500 Internal Server Error):**

```json
{
  "success": false,
  "message": "device 0000:01:00.0 not bound to vfio-pci"
}
```

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/unbind \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'
```

---

### Get VFIO Group Paths

Get `/dev/vfio/GROUP_NUMBER` paths for specified devices.

**Endpoint:** `POST /api/v1/vfio/devices/group-paths`

**Request Body:**

```json
{
  "pci_addresses": ["0000:01:00.0", "0000:01:00.1"]
}
```

**Response (200 OK):**

```json
{
  "group_paths": ["/dev/vfio/1"]
}
```

**Fields:**

- `group_paths` - Array of VFIO group device paths that can be passed to hypervisors

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/group-paths \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'
```

---

## Common Workflows

### 1. Discover and Validate GPU

```bash
# Get device information
curl -X POST http://localhost:8080/api/v1/vfio/devices/info \
  -H "Content-Type: application/json" \
  -d '{"pci_address": "0000:01:00.0"}'

# Check IOMMU group
curl -X POST http://localhost:8080/api/v1/vfio/devices/iommu-groups \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'

# Validate device
curl -X POST http://localhost:8080/api/v1/vfio/devices/validate \
  -H "Content-Type: application/json" \
  -d '{
    "pci_addresses": ["0000:01:00.0"],
    "allowlist": ["10de:*"]
  }'
```

### 2. Bind GPU for Passthrough

```bash
# Bind device to vfio-pci
curl -X POST http://localhost:8080/api/v1/vfio/devices/bind \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'

# Verify binding
curl -X POST http://localhost:8080/api/v1/vfio/devices/info \
  -H "Content-Type: application/json" \
  -d '{"pci_address": "0000:01:00.0"}'
# Should show "driver": "vfio-pci"
```

### 3. Clean Up After Use

```bash
# Unbind device from vfio-pci
curl -X POST http://localhost:8080/api/v1/vfio/devices/unbind \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'
```

---

## Error Handling

All endpoints return standard HTTP status codes:

- `200 OK` - Request successful
- `400 Bad Request` - Invalid request body or parameters
- `404 Not Found` - Device not found
- `500 Internal Server Error` - Server-side error (check logs)

Error responses include a JSON body with an `error` field:

```json
{
  "error": "PCI device not found: 0000:01:00.0"
}
```

---

## Security Considerations

1. **Elevated Privileges Required** - Device binding/unbinding requires root access
2. **API Key Authentication** - Set `VOLANT_API_KEY` environment variable to secure endpoints
3. **IP Allowlisting** - Use `VOLANT_API_ALLOW_CIDR` to restrict API access
4. **Device Allowlisting** - Always use allowlist patterns to restrict which devices can be passed through

**Example with Security:**

```bash
export VOLANT_API_KEY="your-secret-key"
export VOLANT_API_ALLOW_CIDR="192.168.1.0/24,10.0.0.0/8"

curl -X POST http://localhost:8080/api/v1/vfio/devices/validate \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-key" \
  -d '{
    "pci_addresses": ["0000:01:00.0"],
    "allowlist": ["10de:2204"]
  }'
```

---

## OpenAPI Specification

The full OpenAPI specification for these endpoints is available at:

```
GET http://localhost:8080/openapi
```

Import this into Swagger UI, Postman, or any OpenAPI-compatible tool for interactive API exploration.

---

## Related Documentation

- [GPU Passthrough Guide](../3_guides/4_gpu-passthrough.md) - Complete guide to GPU passthrough
- [VM Configuration Reference](./1_manifest-schema.md) - VM manifest device configuration
- [HTTP API Reference](./2_http-api.md) - General API documentation

---

## Troubleshooting

### Device Not Found

**Error:** `PCI device not found: 0000:01:00.0`

**Solution:**
```bash
# List all PCI devices
lspci -nn

# Verify device exists in sysfs
ls /sys/bus/pci/devices/0000:01:00.0
```

### Device Binding Failed

**Error:** `device 0000:01:00.0 is in use by host`

**Solution:**
```bash
# Check current driver
lspci -k -s 0000:01:00.0

# Manually unbind from current driver
echo "0000:01:00.0" | sudo tee /sys/bus/pci/drivers/nvidia/unbind
```

### IOMMU Not Enabled

**Error:** `IOMMU not enabled on this system`

**Solution:**
1. Enable VT-d (Intel) or AMD-Vi (AMD) in BIOS
2. Add kernel boot parameters:
   - Intel: `intel_iommu=on iommu=pt`
   - AMD: `amd_iommu=on iommu=pt`
3. Reboot and verify with `dmesg | grep -i iommu`

### Permission Denied

**Error:** `permission denied`

**Solution:**
- Ensure Volant server is running with root/sudo privileges
- Check file permissions on `/sys/bus/pci/devices/` and `/dev/vfio/`
