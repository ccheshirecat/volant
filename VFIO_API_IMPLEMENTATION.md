# VFIO Device Management API Implementation

## Summary

This document describes the HTTP API endpoints that have been added to Volant for managing VFIO (Virtual Function I/O) device passthrough. These endpoints provide programmatic access to device discovery, validation, binding, and unbinding operations.

## What Was Added

### 1. HTTP API Endpoints (6 endpoints)

All endpoints are under `/api/v1/vfio`:

1. **POST /api/v1/vfio/devices/info** - Get detailed device information
2. **POST /api/v1/vfio/devices/validate** - Validate PCI addresses and allowlist
3. **POST /api/v1/vfio/devices/iommu-groups** - Check IOMMU group membership
4. **POST /api/v1/vfio/devices/bind** - Bind devices to vfio-pci driver
5. **POST /api/v1/vfio/devices/unbind** - Unbind devices from vfio-pci driver
6. **POST /api/v1/vfio/devices/group-paths** - Get /dev/vfio/GROUP paths

### 2. Request/Response Types

Added the following types to `internal/server/httpapi/httpapi.go`:

- `vfioDeviceInfoRequest` / `vfioDeviceInfoResponse`
- `vfioValidateRequest` / `vfioValidateResponse`
- `vfioIOMMUGroupResponse`
- `vfioBindRequest` / `vfioBindResponse`
- `vfioUnbindRequest` / `vfioUnbindResponse`
- `vfioGroupPathsRequest` / `vfioGroupPathsResponse`

### 3. Handler Methods

Six new handler methods added to `apiServer`:

- `getVFIODeviceInfo()`
- `validateVFIODevices()`
- `checkVFIOIOMMUGroups()`
- `bindVFIODevices()`
- `unbindVFIODevices()`
- `getVFIOGroupPaths()`

### 4. OpenAPI Specification

All 6 endpoints have been fully documented in the OpenAPI spec with:
- Request/response schemas
- Descriptions and operation IDs
- Error responses (400, 500)
- Tagged under "vfio"

### 5. Documentation

Created comprehensive API documentation at `docs/6_reference/3_vfio-api.md` including:
- Endpoint descriptions
- Request/response examples
- Common workflows
- Security considerations
- Troubleshooting guide

## Files Modified

1. **internal/server/httpapi/httpapi.go**
   - Added devicemanager import
   - Added 6 VFIO route registrations in the router
   - Added 10 request/response type definitions
   - Added 6 handler method implementations (~160 lines)

2. **internal/server/httpapi/openapi.go**
   - Added OpenAPI schema registrations for all VFIO types
   - Added 6 complete operation definitions with request/response schemas

3. **docs/6_reference/3_vfio-api.md** (NEW)
   - Complete API reference documentation (447 lines)

## Architecture

The implementation follows the existing Volant API patterns:

```
HTTP Request
    ↓
Gin Router (/api/v1/vfio/*)
    ↓
Handler Method (api.getVFIODeviceInfo, etc.)
    ↓
VFIOManager (devicemanager.NewVFIOManager)
    ↓
Filesystem Operations (/sys/bus/pci/devices/, /dev/vfio/)
```

Each handler:
1. Validates the request body using Gin's binding
2. Creates a VFIOManager instance
3. Calls the appropriate VFIOManager method
4. Returns JSON response with success/error information

## Example Usage

### Get Device Info
```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/info \
  -H "Content-Type: application/json" \
  -d '{"pci_address": "0000:01:00.0"}'
```

Response:
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

### Validate Devices
```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/validate \
  -H "Content-Type: application/json" \
  -d '{
    "pci_addresses": ["0000:01:00.0"],
    "allowlist": ["10de:*"]
  }'
```

Response:
```json
{
  "valid": true,
  "message": "All devices are valid and available for passthrough"
}
```

### Bind Device
```bash
curl -X POST http://localhost:8080/api/v1/vfio/devices/bind \
  -H "Content-Type: application/json" \
  -d '{"pci_addresses": ["0000:01:00.0"]}'
```

Response:
```json
{
  "success": true,
  "message": "Devices successfully bound to vfio-pci driver",
  "bound_devices": ["0000:01:00.0"]
}
```

## Integration with Existing VFIO System

The API endpoints leverage the existing VFIOManager implementation:

- **VFIOManager** (`internal/server/devicemanager/vfio_manager.go`) - Already implemented
- **Orchestrator Integration** - Already uses VFIOManager during VM creation
- **Device Configuration** - Already supported via `pluginspec.DeviceConfig`

The new API simply exposes these internal capabilities via HTTP endpoints.

## Security Considerations

1. **Root Privileges Required** - Device binding/unbinding operations require elevated privileges
2. **API Authentication** - Use `VOLANT_API_KEY` environment variable
3. **IP Allowlisting** - Use `VOLANT_API_ALLOW_CIDR` to restrict access
4. **Device Allowlisting** - Always validate against allowlist patterns

## Testing

The implementation:
- ✅ Compiles without errors
- ✅ Passes `gofmt` formatting
- ✅ No diagnostic errors or warnings
- ✅ Follows existing API patterns
- ✅ Fully documented in OpenAPI spec

To test with a running Volant server:
```bash
# Ensure server is running with root privileges
sudo volant server start

# Test device info endpoint
curl -X POST http://localhost:8080/api/v1/vfio/devices/info \
  -H "Content-Type: application/json" \
  -d '{"pci_address": "0000:01:00.0"}'
```

## OpenAPI Specification

The OpenAPI spec is available at:
```
GET http://localhost:8080/openapi
```

Import into Swagger UI, Postman, or any OpenAPI tool for interactive API exploration.

## Related Documentation

- [GPU Passthrough Guide](docs/3_guides/4_gpu-passthrough.md)
- [VFIO API Reference](docs/6_reference/3_vfio-api.md)
- [Manifest Schema](docs/6_reference/1_manifest-schema.md)

## Next Steps

To use these endpoints:

1. Ensure IOMMU is enabled on the host system
2. Start Volant server with root privileges
3. Use the API to discover and manage devices
4. Create VMs with device passthrough via the manifest

Example VM creation with VFIO:
```json
{
  "name": "gpu-vm",
  "plugin": "pytorch",
  "cpu_cores": 8,
  "memory_mb": 16384,
  "config": {
    "manifest": {
      "devices": {
        "pci_passthrough": ["0000:01:00.0"],
        "allowlist": ["10de:*"]
      }
    }
  }
}
```

## Implementation Details

- **No Breaking Changes** - All additions are new endpoints
- **Backward Compatible** - Existing API remains unchanged
- **Consistent Patterns** - Follows Gin router and handler conventions
- **Comprehensive Docs** - Full API reference and examples provided
- **OpenAPI Compliant** - Auto-generated schemas for all types

## Summary

The VFIO device management API is now complete and ready for use. It provides programmatic access to all VFIO operations needed for GPU and hardware passthrough, with comprehensive documentation and OpenAPI specification support.