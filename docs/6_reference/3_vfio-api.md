# VFIO API reference (overview)

Ground truth endpoints are defined in internal/server/httpapi/httpapi.go and the OpenAPI generator. For full, machine-readable detail, generate docs/api-reference/openapi.json via `make openapi-export`.

## Endpoints (all under /api/v1/vfio)

- POST /devices/info — get device information (PCI IDs, IOMMU group, NUMA, driver)
- POST /devices/validate — validate PCI addresses against optional allowlist
  - Request: { "pci_addresses": ["0000:01:00.0"], "allowlist": ["10de:*"] }
- POST /devices/iommu-groups — list IOMMU groups for devices
  - Request: { "pci_addresses": ["..."] }
- POST /devices/bind — bind devices to vfio-pci
  - Request: { "pci_addresses": ["..."] }
- POST /devices/unbind — unbind devices from vfio-pci
  - Request: { "pci_addresses": ["..."] }
- POST /devices/group-paths — resolve /dev/vfio/<group> paths for use by the runtime
  - Request: { "pci_addresses": ["..."] }

Notes:
- Requires Linux with IOMMU enabled and vfio-pci driver available.
- When a VM with passthrough devices is created, Volant validates, binds, and injects the VFIO group device paths into the runtime spec. On VM deletion, devices are unbound (best-effort).

For schema details (request/response types and error shapes), see the generated OpenAPI document.
