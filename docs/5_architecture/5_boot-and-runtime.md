# Boot and Runtime

## Kernel Selection

References:
- internal/server/orchestrator/cloudhypervisor/launcher.go (kernelSrc selection)

- Launcher preference:
  - If LaunchSpec.KernelOverride set → use that path
  - Else if Initramfs present → use vmlinux (uncompressed)
  - Else → use bzImage (compressed)

## Boot Media

- Initramfs
  - Manifest.Initramfs.url → required
  - Optional checksum (sha256:...)
  - Fastest boot path; pairs well with kestrel agent

- RootFS
  - Manifest.RootFS.url → required
  - Optional checksum (sha256:...)
  - Attached as writable disk; default device/fstype set when missing (vda/ext4)

## Cloud-Init

- When configured (manifest or overrides), cloud-init NoCloud is built and attached as read-only disk (CIDATA)
- Code: internal/server/orchestrator/cloudinit/builder.go
- Inputs: user-data, meta-data, optional network-config

## Kernel Command Line

- Base args set by orchestrator (ip=... if managed, console, panic, etc.)
- Additional runtime args from pluginspec constants (runtime, api_host, api_port, plugin, encoded manifest)
- Code: orchestrator.go buildKernelCmdline/appendKernelArgs, launcher assembles --cmdline

## Process Model

- Cloud Hypervisor process managed by runtime.Instance
  - Wait channel for exit, graceful termination (SIGTERM, then SIGKILL on timeout)
  - Serial console via UNIX socket per VM
  - Artifacts cleaned on stop (kernel/initramfs/rootfs/serial)
