# Image Pipeline (Preview)

The image build flow produces two artifacts consumed by the orchestrator:

- `vmlinux-x86_64` – downloaded from the Cloud Hypervisor kernel release (default URL baked into the script).
- `viper-initramfs.cpio.gz` – initramfs containing Chrome headless + `viper-agent` and the custom C init.

## Requirements
- Docker (tested with Docker Desktop on macOS/Linux).
- `viper-agent` binary built at `bin/viper-agent`.

## Steps
1. Build the agent: `make build-agent`.
2. Run the helper script:
   ```bash
   ./build/images/build-initramfs.sh [path/to/viper-agent]
   ```
3. Artifacts are written to `build/artifacts/`.

The script:
- Builds the Docker context under `build/images/` which compiles a statically linked `/sbin/viper-init` (C) and copies `viper-agent` into the Chromedp headless-shell base image.
- Exports the container filesystem and packages it into a gzip-compressed `cpio` archive suitable for use as an initramfs.
- Downloads the default Cloud Hypervisor kernel (`vmlinux-x86_64`) if not already present.

## Customisation
- Override environment variables:
  - `IMAGE_TAG` – Docker image tag used for intermediate build.
  - `INITRAMFS_NAME` – output filename for the initramfs.
  - `KERNEL_URL` – alternate kernel download URL.
- Provide a custom agent path as the first argument.

## TODO
- Integrate into the top-level Makefile with dependency checks.
- Support arm64 kernels and multi-arch builds.
- Package checksum metadata alongside artifacts.
