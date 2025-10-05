# Glossary

- Agent (kestrel): In-guest process that acts as PID1 in default initramfs mode and proxies OpenAPI calls over vsock.
- Bridge (vbr0): Linux bridge used to connect VM tap devices to the host network.
- CIDATA: Volume label for NoCloud seed images used by cloud-init.
- Cloud-init: Standard for VM initialization; Volant supports NoCloud via seed image.
- Deployment: A named set of identical VM replicas managed by the orchestrator.
- Init modes: default/custom/none — control PID1 behavior in initramfs builds.
- Manifest: JSON document describing plugin runtime, boot media, devices, and more.
- Rootfs: A bootable filesystem image derived from an OCI image (ext4/xfs/btrfs).
- Initramfs: Compressed RAM filesystem used as initial userspace.
- Tap: TUN/TAP device used to provide Ethernet to a microVM.
- Vsock: Virtio socket channel used for host↔guest communication without Ethernet.
- VFIO: Linux kernel framework for PCI passthrough; used for GPUs/devices.
