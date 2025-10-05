# GPU Passthrough (VFIO)

This guide is grounded in the implemented VFIO API and device manager:
- internal/server/httpapi/httpapi.go (/api/v1/vfio/*)
- internal/server/httpapi/openapi.go (OpenAPI definitions)
- internal/server/devicemanager/vfio_manager.go (binding, validation)

## Host requirements

- IOMMU enabled in kernel/bootloader
- vfio-pci driver available
- Root privileges to bind/unbind

## API overview

Endpoints (all POST):
- /api/v1/vfio/devices/info
- /api/v1/vfio/devices/validate
- /api/v1/vfio/devices/iommu-groups
- /api/v1/vfio/devices/bind
- /api/v1/vfio/devices/unbind
- /api/v1/vfio/devices/group-paths

See docs/6_reference/3_vfio-api.md for full reference.

## Typical flow

1) Validate the device(s) and allowlist (optional):
```bash
curl -sX POST :8080/api/v1/vfio/devices/validate \
 -H 'Content-Type: application/json' \
 -d '{"pci_addresses":["0000:01:00.0"],"allowlist":["10de:*"]}'
```

2) Bind to vfio-pci:
```bash
curl -sX POST :8080/api/v1/vfio/devices/bind \
 -H 'Content-Type: application/json' \
 -d '{"pci_addresses":["0000:01:00.0"]}'
```

3) Reference in plugin manifest under devices:
```json
{
  "devices": {
    "pci_passthrough": ["0000:01:00.0"],
    "allowlist": ["10de:*"]
  }
}
```

4) Create VM with that plugin and start it.

5) To unbind later:
```bash
curl -sX POST :8080/api/v1/vfio/devices/unbind \
 -H 'Content-Type: application/json' \
 -d '{"pci_addresses":["0000:01:00.0"]}'
```

## Troubleshooting

- Ensure device is in its own IOMMU group or pass through the full group
- Validate group membership via /devices/iommu-groups
- Check that `vfio-pci` is the active driver
- NUMA considerations are exposed by the info endpoint
