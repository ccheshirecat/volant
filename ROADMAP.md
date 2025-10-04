# Roadmap

Volant's development priorities and future direction.

---

## Current State 

**Core functionality is complete:**

- Dual-kernel boot (bzImage for rootfs, vmlinux for initramfs)
- OCI image compatibility (rootfs path)
- Custom initramfs appliances (performance path)
- Static IP management with bridge networking
- Vsock networking
- Plugin registry and manifest system
- REST and MCP APIs
- Deployment orchestration with replica scaling
- Event streaming
- `volantd` control plane, `volar` CLI, `kestrel` agent, `fledge` builder

---

## Next (this or next month)

### VFIO GPU Passthrough

**Priority: Immediate**

Enable direct GPU access for AI/ML workloads:

- Pass dedicated GPUs (NVIDIA/AMD) to individual microVMs
- Full CUDA/ROCm support for ML inference
- Hardware acceleration for rendering and compute
- Isolation guarantees for multi-tenant GPU workloads

**Use Cases:**
- ML model inference with GPU acceleration
- AI workloads requiring dedicated hardware
- GPU-accelerated rendering and video encoding

---

## Future (planned to be in development by EOY)

### PaaS Mode

Simple `git push` deployment workflow:

- Heroku/Vercel-style developer experience
- Automatic builds from Git repositories
- Built-in reverse proxy and SSL
- Zero-configuration deployments

### Snapshot/Restore

Instant cold-start for serverless-style workloads:

- Snapshot running VMs to disk
- Restore from snapshot in milliseconds
- Pre-warmed workloads for faster response times
- Ideal for function-as-a-service patterns

---

## License

**Business Source License 1.1**

Volant is open source under BSL 1.1, which allows free use for non-production purposes. The license converts to Apache 2.0 on **October 4, 2029**.

See [LICENSE](LICENSE) for full terms.

---

*Volant ships fast and stays focused.*
