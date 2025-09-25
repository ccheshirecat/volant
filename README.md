<p align="center">
  <img src="banner.png" alt="VOLANT — The Intelligent Execution Cloud"/>
</p>

<p align="center">
  <a href="https://github.com/ccheshirecat/volant/actions">
    <img src="https://img.shields.io/github/actions/workflow/status/ccheshirecat/volant/ci.yml?branch=main&style=flat-square&label=tests" alt="Build Status">
  </a>
  <a href="https://github.com/ccheshirecat/volant/releases">
    <img src="https://img.shields.io/github/v/release/ccheshirecat/volant.svg?style=flat-square" alt="Latest Release">
  </a>
  <a href="https://golang.org/">
    <img src="https://img.shields.io/badge/Go-1.22+-black.svg?style=flat-square" alt="Go Version">
  </a>
  <a href="https://github.com/ccheshirecat/volant/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/License-Apache_2.0-black.svg?style=flat-square" alt="License">
  </a>
</p>

---

# VOLANT

> **The Modular Engine for Modern Browser Automation**  
> Secure. Stateful. Scalable. Powered by **microVMs.**

---

## ⚡ What is Volant?

Volant is a **minimal, opinionated execution layer** designed to make  
headless browser automation effortless at scale.  

With a single command, you get:  
- Bridged networking  
- MicroVM orchestration  
- Stateful isolation  
- Infinite horizontal scaling  

All inside **ephemeral initramfs-based microVMs** tuned for speed and stealth.  

---

## 🌌 Why Volant?

Volant is built for one thing:  
to make **browser automation** secure, scalable, and effortless.  

Most existing approaches lean on containers or fragile hacks.  
We went lower: every browser runs inside a **minimal microVM** with:  
- Native networking  
- Kernel-level isolation  
- Built-in orchestration  

No extra setup. No Kubernetes. No boilerplate.  
Just the cleanest way to run browsers at scale — the way it should have always been.  

While Volant is purpose-built for browsers, it’s not an accident.  
It reflects the same design philosophy that guides our broader work:  
**simple, opinionated systems that scale without complexity.**

---

## 🚀 Quickstart

```bash
# Install Volant instantly
# Downloads binaries, builds initramfs using docker, and downloads kernel

curl -sSL https://install.volant.cloud | bash

# Set up systemd service and configure bridge networking

sudo volar setup 

# Launch the TUI dashboard
volar

# Create your first microVM
volar vms create my-first-vm
```

---

## ✨ Features

- 🛡 **Security by Design** — Kernel-level isolation for every workload.  
- 💾 **Stateful Persistence** — Snapshot + restore environments instantly.  
- ⚡ **Velocity Through Simplicity** — Zero-config setup + intuitive TUI.  
- 🌐 **Universal Architecture** — Orchestrate browsers, AI inference, anything.  
- 🤖 **AI-Native** — MCP + AG-UI protocols built in.  

---

## 🔥 Example: Browser Automation

```bash
# Install the browser automation plugin
volar plugins install viper

# Create a fleet of 10 browser microVMs
volar workloads create browser-farm   --plugin viper   --profiles ./profiles/accounts.json   --count 10

# Execute a high-level action
volar workloads action browser-farm navigate --url="https://example.com"
```

---

## 📂 Repository Layout

<details>
<summary>Expand</summary>

```

cmd/              # Entry points
volantd/        # Control plane daemon
volar/          # CLI client
volary/         # In-VM agent

internal/         # Core implementation
agent/          # Agent runtime
cli/            # Cobra CLI + Bubble Tea TUI
protocol/       # MCP + AG-UI integrations
server/         # Orchestrator, API, persistence
app/          # Application lifecycle
config/       # Config loading + validation
db/           # SQLite persistence
eventbus/     # Internal pub/sub
httpapi/      # REST + SSE endpoints
orchestrator/ # MicroVM scheduling
setup/          # Host initialization
shared/         # Logging, helpers, common utils

build/            # Image pipeline + artifacts
artifacts/
images/

Makefile          # Build + setup automation
go.mod / go.sum   # Dependencies
README.md         # This file

docs/             # Documentation
```
</details>

---

## 🧩 Development Setup

<details>
<summary>Expand</summary>

1. Install **Go 1.22+**.  
2. Export environment variables:  
   - `VOLANT_KERNEL` — path to kernel image  
   - `VOLANT_INITRAMFS` — path to initramfs bundle  
   - `VOLANT_HOST_IP` — default: `192.168.127.1`  
   - `VOLANT_RUNTIME_DIR` / `VOLANT_LOG_DIR` — sockets & logs  
3. Build binaries:  
   ```bash
   make build  
   ```  
4. Run dry-run host config:  
   ```bash
   volar setup --dry-run
   sudo volar setup
   ```  
5. Build initramfs + kernel:  
   ```bash
   ./build/images/build-initramfs.sh
   ```
</details>

---

## 📖 Documentation

→ [**docs.volant.cloud**](https://docs.volant.cloud) for guides, deep dives, and API reference.  

---

## 🤝 Contributing

Contributions are welcome. PRs encouraged.  

---

## 📜 License

Licensed under **Apache 2.0**. See [LICENSE](LICENSE).

---

<p align="center"><i>Volant — Defined by design, not discussion.</i></p>
