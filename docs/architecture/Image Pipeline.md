# Image Pipeline

Volant supports dual boot modes:

- bzImage + rootfs: Default Docker-friendly mode. A Cloud Hypervisor compatible bzImage embeds our initramfs and the `volary` agent; plugins provide an external root filesystem (read-only squashfs or writable ext4/raw) attached as a virtio disk.
- vmlinux + initramfs: High-performance native mode. A raw vmlinux image is paired with a plugin-provided initramfs, enabling ultra-fast boot and minimal I/O, ideal for native plugins that don't need a disk rootfs.

The orchestrator no longer expects plugins to bring a kernel; plugins specify exactly one of `rootfs` or `initramfs`. Operators can override boot media per-VM.

This document walks through the artifacts we produce, how the host hydrates plugin images, and what the in-VM bootstrap does with them.

## Artifact Layout
- `kernels/<arch>/bzImage` — Cloud Hypervisor compatible kernel with embedded initramfs committed to the repository for downloads (per release tag). The default runtime path used by the daemon is `kernel/bzImage` relative to the systemd WorkingDirectory. With WorkingDirectory=/var/lib/volant, that resolves to `/var/lib/volant/kernel/bzImage`.
- `kernels/<arch>/vmlinux` — Raw vmlinux for use with plugin-provided initramfs. Installed to `/var/lib/volant/kernel/vmlinux` when available.
- `checksums.txt` — SHA256 digests for release validation.

Releases publish the `kernels/<arch>/bzImage` and `kernels/<arch>/vmlinux` paths so the installer can fetch them. Local rebuilds are supported via `build/bake.sh` (initramfs only) and the Cloud Hypervisor Linux kernel with `CONFIG_INITRAMFS_SOURCE` for bzImage embedding.

## Host Boot Flow
1. **Manifest install** – When a plugin manifest is registered, it is persisted and cached by the engine (`internal/server/plugins/registry.go`). The manifest declares a `rootfs` URL, checksum, resource envelope, and action map.
2. **Create VM** – On `createVM`, the orchestrator resolves the manifest and assembles a `LaunchSpec`. Core kernel args always include the IP lease and the identifiers `volant.runtime`, `volant.plugin`, `volant.api_host`, and `volant.api_port` so the agent can crawl back to the control plane. Per‑VM overrides can specify a `kernel_override`, or switch between `initramfs` and `rootfs` boot media.
3. **Boot media hydration** – If `rootfs.url` is set, `cloudhypervisor.Launcher` streams it into the runtime directory before boot. HTTP(S), `file://`, and absolute paths are supported. If `rootfs.checksum` is present, it is verified as a `sha256` (with or without the `sha256:` prefix). If `initramfs.url` is set instead, it is staged and passed to the hypervisor as `--initramfs` along with the `vmlinux` kernel.
4. **Launching Cloud Hypervisor** – The launcher selects `bzImage` when `rootfs` is used, and `vmlinux` when `initramfs` is used. With `rootfs`, the disk is attached as a virtio‑blk device; with `initramfs`, no disk is required.
5. **In-VM init** – Our tiny C init (`build/init.c`) configures the console, brings up `/dev`, `/proc`, `/sys`, and `/run`, and then hydrates the runtime. If a rootfs image was declared it is mounted (loopback or squashfs depending on the build), `stage-volary.sh` copies the agent into `/usr/local/bin`, and control pivots into the plugin filesystem via `switch_root`. Should mounting fail, the initramfs copy of `volary` is used as a safe fallback.
6. **Agent startup** – `volary` reads the kernel command line, fetches the manifest over HTTP, and starts the runtime-specific router. The registered actions are exposed under `/api/v1/plugins/{plugin}/actions/{action}` for legacy compatibility; new plugins should expose their own HTTP/OpenAPI surfaces.

```mermaid
graph TD
    A[Manifest installed] --> B[Create VM request]
    B --> C[Resolve manifest & rootfs metadata]
    C --> D[Stream rootfs to runtime dir & verify checksum]
    D --> E[Launch Cloud Hypervisor with bzImage + virtio disk]
    E --> F[Init mounts rootfs & stages volary]
    F --> G[Agent exposes manifest-defined routes]
```

## Manifest Responsibilities
Plugin authors focus on their runtime artifacts and HTTP contract. A minimal rootfs-based manifest looks like:

```json
{
  "schema_version": "2024-09",
  "name": "browser",
  "version": "2.0.0",
  "runtime": "browser",
  "resources": { "cpu_cores": 2, "memory_mb": 2048 },
  "rootfs": {
    "url": "https://artifacts.example.com/browser/rootfs.squashfs",
    "checksum": "sha256:2d4f41..."
  },
  "workload": {
    "type": "http",
    "base_url": "http://127.0.0.1:8080"
  },
  "actions": {
    "navigate": {
      "description": "Navigate to URL",
      "method": "POST",
      "path": "/v1/browser/navigate",
      "timeout_ms": 60000
    }
  },
  "openapi": "https://artifacts.example.com/browser/openapi.json",
  "labels": { "tier": "reference" }
}
```

- `rootfs.url` accepts HTTP(S), `file://`, or absolute host paths. For initramfs mode, specify `initramfs.url` instead. Exactly one of `rootfs` or `initramfs` must be provided.
- `rootfs.checksum` is optional but recommended. Provide a bare SHA256 or prefix it with `sha256:`.
- `actions` must point at real HTTP endpoints inside the VM. The control plane proxies requests verbatim, so the manifest’s OpenAPI document should describe the same surface.

## Rootfs Expectations
- Include the binaries, assets, and services your runtime needs. At minimum, provide the HTTP routes declared in the manifest.
- Placing `volary` in the image is optional; `stage-volary.sh` copies the embedded agent into `/usr/local/bin` before the switch_root happens.
- Network is preconfigured by the kernel parameters, so services can bind to `0.0.0.0` or `127.0.0.1` immediately.
- SquashFS images work well for read-only runtimes, but writable formats (ext4 raw disks) are also supported because the disk is attached as a standard virtio-blk device.

## Building Rootfs Images
The Volant repo does not bundle a generic rootfs build system, but the companion [`fsify`](https://github.com/ccheshirecat/fsify) tool converts OCI/Docker images into bootable filesystem artifacts (squashfs, ext4, raw disks). A typical workflow is:

```bash
# Convert an OCI image to a squashfs rootfs and publish it
fsify convert oci://ghcr.io/acme/browser:2.0 --format squashfs --output ./rootfs.squashfs
sha256sum ./rootfs.squashfs
```

Upload the resulting artifact to your distribution channel, record the SHA256 in the manifest, and the engine takes care of staging it for each VM.

## Injecting files into the initramfs (development)

Use the bake script to add files during initramfs build:

```
./build/bake.sh --copy ./extras/startup.sh:/usr/local/bin/startup.sh \
                --copy ./config/app.yaml:/etc/app.yaml
```

The script preserves mtimes with `SOURCE_DATE_EPOCH` and strips gzip timestamps for reproducibility.

## Troubleshooting
- **Rootfs fetch failures** – Verify the URL is reachable from the host and the checksum matches. Errors are surfaced from `cloudhypervisor.Launcher` before boot.
- **Manifest fetch failures inside the VM** – Ensure `volant.api_host`, `volant.api_port`, and `volant.plugin` were set. The control plane automatically injects them, but custom kernel flags can accidentally shadow them.
- **Agent fallback** – If your image does not contain `volary`, the initramfs copy will run instead. Check the serial log (`~/.volant/logs/<vm>.log`) for mount errors.
