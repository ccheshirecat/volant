# Building from Source

This guide provides detailed instructions for building Volant from source code.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Detailed Build Steps](#detailed-build-steps)
- [Build Targets](#build-targets)
- [Cross-Compilation](#cross-compilation)
- [Development Builds](#development-builds)
- [Installing](#installing)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Required Software

| Software | Minimum Version | Purpose |
|----------|----------------|---------|
| **Go** | 1.24.0+ | Compile Volant binaries |
| **Make** | Any | Build automation |
| **Git** | Any | Clone repository |
| **GCC** | Any | CGo compilation |

### Required for Runtime

| Software | Purpose |
|----------|---------|
| **QEMU** | Run microVMs |
| **Linux** | Host operating system |

### Supported Platforms

**Build Platforms**:
- Linux (x86_64, arm64)
- macOS (x86_64, arm64) - build only, not runtime
- Windows (x86_64) via WSL - build only, not runtime

**Runtime Platforms**:
- Linux x86_64 with KVM support

---

## Quick Start

```bash
# Clone repository
git clone https://github.com/volantvm/volant.git
cd volant

# Install dependencies
make tidy

# Build all binaries
make build

# Binaries are in ./bin/
ls -lh bin/
# volantd - Server daemon
# kestrel - Guest agent
# volar   - CLI tool
```

---

## Detailed Build Steps

### 1. Install Go

#### Ubuntu/Debian

```bash
# Remove old Go installations
sudo rm -rf /usr/local/go

# Download Go 1.24+
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz

# Extract to /usr/local
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
```

#### Fedora/RHEL

```bash
# Use dnf
sudo dnf install golang

# Or install manually from golang.org (for latest version)
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

#### macOS

```bash
# Using Homebrew
brew install go

# Or download from golang.org
wget https://go.dev/dl/go1.24.0.darwin-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.darwin-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
source ~/.zshrc
```

### 2. Install Build Tools

#### Ubuntu/Debian

```bash
sudo apt-get update
sudo apt-get install -y \
  build-essential \
  make \
  git
```

#### Fedora/RHEL

```bash
sudo dnf groupinstall "Development Tools"
sudo dnf install make git
```

### 3. Clone Repository

```bash
# Via HTTPS
git clone https://github.com/volantvm/volant.git
cd volant

# Via SSH (if you have GitHub SSH keys)
git clone git@github.com:volantvm/volant.git
cd volant
```

### 4. Install Go Dependencies

```bash
# Download and sync dependencies
make tidy

# Or manually
go mod download
go mod tidy
```

### 5. Build Binaries

```bash
# Build all binaries (volantd, kestrel, volar)
make build

# Build specific components
make build-server   # Build volantd only
make build-agent    # Build kestrel only
make build-cli      # Build volar only
```

**Output Location**: `./bin/`

```bash
$ ls -lh bin/
-rwxr-xr-x 1 user user  45M Jan 15 10:00 volantd
-rwxr-xr-x 1 user user  12M Jan 15 10:00 kestrel
-rwxr-xr-x 1 user user  18M Jan 15 10:00 volar
```

### 6. Verify Build

```bash
# Check volantd
./bin/volantd --version

# Check kestrel
./bin/kestrel --version

# Check volar
./bin/volar version
```

---

## Build Targets

The `Makefile` provides several targets:

### Core Builds

```bash
# Build all binaries
make build

# Build server (volantd)
make build-server

# Build guest agent (kestrel)
make build-agent

# Build CLI (volar)
make build-cli

# Build OpenAPI export utility
make build-openapi-export
```

### Installation

```bash
# Install binaries to /usr/local/bin (requires sudo)
sudo make install

# Install to custom directory
sudo make install INSTALL_DIR=/opt/volant/bin
```

### Testing

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/server/orchestrator/...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Code Quality

```bash
# Format code
make fmt

# Run go vet
make vet

# Run all CI checks (fmt + vet + test)
make ci
```

### Maintenance

```bash
# Clean build artifacts
make clean

# Sync dependencies
make tidy
```

### Help

```bash
# Show all available targets
make help
```

---

## Cross-Compilation

### Build for Different Architectures

```bash
# Build for ARM64
GOARCH=arm64 make build

# Build for specific OS/architecture
GOOS=linux GOARCH=amd64 make build

# Build for multiple platforms
for arch in amd64 arm64; do
  GOARCH=$arch make build
  mv bin/volantd bin/volantd-$arch
  mv bin/kestrel bin/kestrel-$arch
  mv bin/volar bin/volar-$arch
done
```

### Supported Combinations

| OS | Architecture | Supported |
|----|--------------|-----------|
| linux | amd64 |  Full support |
| linux | arm64 |  Full support |
| darwin | amd64 |  Build only |
| darwin | arm64 |  Build only |
| windows | amd64 | ❌ Not supported |

**Note**: Runtime support is Linux-only. macOS and Windows can build binaries but cannot run volantd.

---

## Development Builds

### Debug Builds

```bash
# Build with debug symbols
go build -gcflags="all=-N -l" -o bin/volantd ./cmd/volantd

# Or use Go's race detector
go build -race -o bin/volantd ./cmd/volantd
```

### Development Mode

```bash
# Build and run server in one step
go run ./cmd/volantd/main.go

# Build and run CLI
go run ./cmd/volar/main.go vms list
```

### Custom Build Flags

```bash
# Add version information
VERSION=$(git describe --tags --always --dirty)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT=$(git rev-parse --short HEAD)

go build \
  -ldflags "-X main.Version=$VERSION -X main.BuildDate=$BUILD_DATE -X main.Commit=$COMMIT" \
  -o bin/volantd \
  ./cmd/volantd
```

### Optimized Builds

```bash
# Reduce binary size
go build \
  -ldflags="-s -w" \
  -trimpath \
  -o bin/volantd \
  ./cmd/volantd

# Further compression with UPX
upx --best --lzma bin/volantd
```

---

## Installing

### System-Wide Installation

```bash
# Build binaries
make build

# Install to /usr/local/bin (default)
sudo make install

# Install to custom location
sudo make install INSTALL_DIR=/opt/volant/bin
```

### Manual Installation

```bash
# Build binaries
make build

# Copy to system path
sudo cp bin/volantd /usr/local/bin/
sudo cp bin/kestrel /usr/local/bin/
sudo cp bin/volar /usr/local/bin/

# Set executable permissions
sudo chmod +x /usr/local/bin/volantd
sudo chmod +x /usr/local/bin/kestrel
sudo chmod +x /usr/local/bin/volar
```

### User-Only Installation

```bash
# Create local bin directory
mkdir -p ~/.local/bin

# Copy binaries
cp bin/* ~/.local/bin/

# Add to PATH (if not already)
echo 'export PATH=$HOME/.local/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
```

### Systemd Service Installation

```bash
# Build and install binaries
make build
sudo make install

# Install systemd service
sudo volar setup systemd

# Or create service manually
sudo tee /etc/systemd/system/volantd.service > /dev/null <<EOF
[Unit]
Description=Volant VM Orchestrator
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/volantd
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable volantd
sudo systemctl start volantd
```

---

## Troubleshooting

### Build Errors

#### "go: command not found"

**Solution**: Install Go or add to PATH

```bash
export PATH=$PATH:/usr/local/go/bin
source ~/.bashrc
```

#### "package X is not in GOROOT"

**Solution**: Download dependencies

```bash
go mod download
go mod tidy
```

#### "gcc: command not found"

**Solution**: Install build tools

```bash
# Ubuntu/Debian
sudo apt-get install build-essential

# Fedora
sudo dnf groupinstall "Development Tools"
```

### Runtime Errors

#### "permission denied" when starting volantd

**Solution**: Run with sudo or add user to kvm group

```bash
# Option 1: Run with sudo
sudo volantd

# Option 2: Add user to kvm group
sudo usermod -aG kvm $USER
newgrp kvm
```

#### "cannot access /dev/kvm"

**Solution**: Enable KVM

```bash
# Check if KVM is available
ls -l /dev/kvm

# Load KVM module
sudo modprobe kvm
sudo modprobe kvm-intel  # For Intel CPUs
sudo modprobe kvm-amd    # For AMD CPUs

# Make permanent
echo "kvm" | sudo tee -a /etc/modules
echo "kvm-intel" | sudo tee -a /etc/modules  # or kvm-amd
```

#### "qemu-system-x86_64: command not found"

**Solution**: Install QEMU

```bash
# Ubuntu/Debian
sudo apt-get install qemu-kvm qemu-system-x86

# Fedora
sudo dnf install qemu-kvm
```

### Build Performance

#### Slow Builds

**Solution**: Use build cache and parallel compilation

```bash
# Enable build cache (default in Go 1.10+)
export GOCACHE=$HOME/.cache/go-build

# Use parallel builds
go build -p $(nproc) ./...

# Use faster linker
go build -ldflags="-linkmode external -extldflags=-Wl,-O1" ./...
```

#### Out of Memory

**Solution**: Limit parallel jobs

```bash
# Reduce parallel compilation
go build -p 2 ./...

# Or set GOMAXPROCS
GOMAXPROCS=2 go build ./...
```

---

## Build Variables

You can customize the build with environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `GO` | `go` | Go compiler command |
| `BIN_DIR` | `bin` | Output directory for binaries |
| `INSTALL_DIR` | `/usr/local/bin` | Installation directory |
| `GOARCH` | host arch | Target architecture |
| `GOOS` | host OS | Target operating system |

**Example**:

```bash
# Custom build directory
make build BIN_DIR=./dist

# Install to /opt
sudo make install INSTALL_DIR=/opt/volant/bin
```

---

## Verifying the Build

### Check Binary Information

```bash
# File type
file bin/volantd

# Size
ls -lh bin/volantd

# Dependencies (Linux)
ldd bin/volantd

# Build info
go version -m bin/volantd
```

### Run Tests

```bash
# Unit tests
make test

# Integration tests (requires QEMU)
go test -tags=integration ./...

# Specific test
go test -v ./internal/server/orchestrator/ -run TestVMCreate
```

### Smoke Test

```bash
# Start server in background
./bin/volantd &
VOLANTD_PID=$!

# Wait for startup
sleep 2

# Check if running
curl http://localhost:7777/api/v1/system/info

# Cleanup
kill $VOLANTD_PID
```

---

## Next Steps

- **[Contributing Guide](1_contributing.md)** – How to contribute to Volant
- **[Installation Guide](../2_getting-started/1_installation.md)** – Install pre-built binaries
- **[Quick Start](../2_getting-started/2_quick-start-rootfs.md)** – Get started with Volant
- **[Architecture Overview](../5_architecture/1_overview.md)** – Understand Volant's architecture
