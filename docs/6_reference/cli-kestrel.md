# kestrel (agent) reference

Kestrel is the in-guest agent and default PID1 for initramfs default mode.

Key behavior (see fledge/internal/builder/embed/init.c and agent code):
- When used as PID1, it prepares the guest environment and exposes a control socket over vsock.
- It can proxy HTTP requests from volantd to workloads running in the guest, enabling fully isolated vsock-only deployments.

The agent receives runtime arguments via the kernel cmdline, encoded by the orchestrator, including:
- runtime (pluginspec.RuntimeKey)
- api host/port (pluginspec.APIHostKey/APIPortKey)
- encoded manifest (pluginspec.CmdlineKey)

For most users, there are no CLI flags to pass directly to kestrel; configuration is provided through the manifest and per-VM config.
