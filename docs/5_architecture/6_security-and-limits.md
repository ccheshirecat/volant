# Security, Limits, and Failure Modes

## Security Model

- Isolation: Cloud Hypervisor microVMs with dedicated kernel per VM
- API protections:
  - VOLANT_API_KEY header (X-Volant-API-Key) or api_key query param
  - VOLANT_API_ALLOW_CIDR to limit incoming clients
- Device passthrough:
  - VFIO flow explicitly validates allowlists and IOMMU groups; devices unbound on VM destroy

## Resource Limits

- Per-VM
  - CPU cores and memory are explicit in VM spec; launcher passes boot=<n>, size=<MB>
  - Disk attachments are explicit; rootfs default writable, additional disks can be readonly

- IPAM
  - Simple in-DB pool derived from configured subnet; excludes network/broadcast/host IP

## Failure Modes

- Launch failures
  - Cloud-init build or media fetch checksum mismatch: VM creation rolled back, artifacts removed
  - Tap/bridge errors: tap cleaned up, DB rolled back

- Agent unavailable
  - Proxy endpoints return 502/503; does not impact VM lifecycle

- Process crashes
  - Instance Wait() monitored; status transitioned to crashed/stopped; events published; taps and artifacts cleaned
