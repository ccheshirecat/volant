# Contributing to Volant

Thank you for your interest in contributing to Volant! This guide will help you understand our development workflow, coding standards, and how to submit contributions.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How to Contribute](#how-to-contribute)
- [Development Setup](#development-setup)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Submitting Changes](#submitting-changes)
- [Documentation](#documentation)
- [Community](#community)

---

## Code of Conduct

Volant is committed to providing a welcoming and inclusive environment for all contributors. We expect all participants to:

- Be respectful and considerate
- Accept constructive criticism gracefully
- Focus on what is best for the community
- Show empathy towards other community members

Unacceptable behavior includes harassment, trolling, derogatory comments, and any form of discrimination.

---

## How to Contribute

There are many ways to contribute to Volant:

### 1. Report Bugs

Found a bug? Please open an issue on GitHub with:

- Clear, descriptive title
- Steps to reproduce the issue
- Expected vs actual behavior
- Volant version (`volar version`, `volantd --version`)
- Operating system and version
- Relevant logs or error messages

**Example Issue Title**: "VM fails to start with bridge networking on Ubuntu 22.04"

### 2. Suggest Features

Have an idea for a new feature? Open a GitHub issue with:

- Clear description of the feature
- Use case and motivation
- Proposed implementation (if you have ideas)
- Examples of similar features in other projects

### 3. Contribute Code

Before starting significant work:

1. **Check existing issues** to avoid duplicate effort
2. **Open a discussion issue** for major changes
3. **Fork the repository** and create a feature branch
4. **Write clean, tested code** following our standards
5. **Submit a pull request** with clear description

### 4. Improve Documentation

Documentation improvements are always welcome:

- Fix typos or clarify confusing sections
- Add examples or use cases
- Translate documentation
- Write guides or tutorials

### 5. Create Plugins

Build plugins for common use cases and share them with the community:

- Package your plugin with Fledge
- Publish plugin manifest
- Add to plugin registry (when available)

---

## Development Setup

### Prerequisites

- **Go 1.24+** (required for Volant server)
- **Linux** (Ubuntu 20.04+, Fedora 35+, or similar)
- **QEMU/KVM** installed
- **Git** for version control
- **Make** for build automation

### Clone the Repository

```bash
# Clone via HTTPS
git clone https://github.com/volantvm/volant.git
cd volant

# Or clone via SSH
git clone git@github.com:volantvm/volant.git
cd volant
```

### Install Dependencies

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y \
  build-essential \
  qemu-kvm \
  qemu-system-x86 \
  libvirt-daemon-system \
  git

# Fedora
sudo dnf install -y \
  make \
  gcc \
  qemu-kvm \
  libvirt \
  git
```

### Install Go

```bash
# Download and install Go 1.24+
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/bin/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify installation
go version
```

### Build from Source

```bash
# Sync Go dependencies
make tidy

# Build all binaries (volantd, kestrel, volar)
make build

# Binaries are created in ./bin/
ls -lh bin/
```

See [Building from Source](2_building-from-source.md) for detailed build instructions.

---

## Coding Standards

### Go Style Guidelines

We follow standard Go conventions:

1. **Formatting**: Use `gofmt` (run `make fmt`)
2. **Linting**: Code must pass `go vet` (run `make vet`)
3. **Naming**: Follow Go naming conventions
   - `CamelCase` for exported identifiers
   - `camelCase` for unexported identifiers
   - Acronyms like `HTTP`, `URL`, `ID` are all caps: `HTTPServer`, `ParseURL`

### Code Organization

```
volant/
├── cmd/                    # Main applications
│   ├── volantd/           # Server entry point
│   ├── volar/             # CLI entry point
│   └── kestrel/           # Guest agent entry point
├── internal/              # Internal packages (not importable)
│   ├── server/            # Server implementation
│   │   ├── httpapi/       # HTTP API handlers
│   │   ├── orchestrator/  # VM orchestration
│   │   └── database/      # Database layer
│   ├── cli/               # CLI implementation
│   └── agent/             # Guest agent implementation
├── pkg/                   # Public packages (importable)
│   └── pluginspec/        # Plugin manifest types
├── docs/                  # Documentation
└── scripts/               # Build/deployment scripts
```

### Error Handling

Always handle errors explicitly:

```go
// Good
vm, err := api.GetVM(ctx, name)
if err != nil {
    return fmt.Errorf("get vm: %w", err)
}

// Bad - swallowing errors
vm, _ := api.GetVM(ctx, name)
```

Use `fmt.Errorf` with `%w` for error wrapping:

```go
if err != nil {
    return fmt.Errorf("failed to start VM %s: %w", name, err)
}
```

### Context Usage

Always pass `context.Context` for operations that:
- Make network calls
- Access databases
- Take significant time
- Should be cancellable

```go
func (s *Server) GetVM(ctx context.Context, name string) (*VM, error) {
    // Use ctx for database queries, API calls, etc.
    return s.db.GetVM(ctx, name)
}
```

### Logging

Use structured logging (we use `slog` or similar):

```go
slog.Info("starting VM",
    "name", vmName,
    "plugin", pluginName,
    "memory_mb", memoryMB)

slog.Error("failed to start VM",
    "name", vmName,
    "error", err)
```

### Comments

- Document all exported functions, types, and constants
- Explain *why*, not *what* (code shows what)
- Use complete sentences with proper punctuation

```go
// GetVM retrieves a VM by name from the database.
// Returns ErrNotFound if the VM doesn't exist.
func GetVM(ctx context.Context, name string) (*VM, error) {
    // Implementation
}
```

---

## Testing Guidelines

### Unit Tests

Write unit tests for all new code:

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/server/orchestrator/...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Structure

Follow Go testing conventions:

```go
func TestVMCreate(t *testing.T) {
    // Setup
    ctx := context.Background()
    db := setupTestDB(t)
    defer db.Close()

    orchestrator := NewOrchestrator(db)

    // Execute
    vm, err := orchestrator.CreateVM(ctx, "test-vm", "nginx-alpine")

    // Assert
    if err != nil {
        t.Fatalf("CreateVM failed: %v", err)
    }
    if vm.Name != "test-vm" {
        t.Errorf("expected name=test-vm, got %s", vm.Name)
    }
}
```

### Table-Driven Tests

Use table-driven tests for multiple scenarios:

```go
func TestValidateManifest(t *testing.T) {
    tests := []struct {
        name    string
        input   *Manifest
        wantErr bool
    }{
        {
            name:    "valid manifest",
            input:   &Manifest{Name: "test", Version: "1.0.0"},
            wantErr: false,
        },
        {
            name:    "missing name",
            input:   &Manifest{Version: "1.0.0"},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateManifest(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateManifest() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Integration Tests

Tag integration tests that require external dependencies:

```go
//go:build integration

func TestVMLifecycle(t *testing.T) {
    // Tests that require QEMU, networking, etc.
}
```

Run with:

```bash
go test -tags=integration ./...
```

### Test Coverage

Aim for:
- **80%+ coverage** for core packages (`internal/server/orchestrator`, `internal/cli/client`)
- **60%+ coverage** overall
- **100% coverage** for public APIs (`pkg/pluginspec`)

---

## Submitting Changes

### Branch Naming

Use descriptive branch names:

```
feature/add-gpu-passthrough
bugfix/fix-tap-device-collision
docs/update-plugin-guide
refactor/simplify-orchestrator
```

### Commit Messages

Follow conventional commits format:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples**:

```
feat(orchestrator): add support for VFIO GPU passthrough

Implements VFIO device passthrough for GPUs, allowing VMs to
access host GPUs directly.

Closes #123
```

```
fix(bridge): prevent TAP device name collisions

Uses hash-based suffixes to ensure unique TAP device names
for deployment replicas.

Fixes #456
```

### Pull Request Process

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make your changes**:
   - Write clean, tested code
   - Update documentation
   - Add tests for new functionality

3. **Run pre-commit checks**:
   ```bash
   make fmt    # Format code
   make vet    # Run go vet
   make test   # Run tests
   make ci     # Run all checks
   ```

4. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat(scope): description"
   ```

5. **Push to your fork**:
   ```bash
   git push origin feature/my-feature
   ```

6. **Open a pull request** on GitHub with:
   - Clear title and description
   - Link to related issues
   - Screenshots/demos for UI changes
   - Test results and coverage
   - Checklist of completed items

### Pull Request Template

```markdown
## Description
Brief description of changes

## Related Issues
Fixes #123
Relates to #456

## Changes Made
- Added X feature
- Fixed Y bug
- Updated Z documentation

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex code
- [ ] Documentation updated
- [ ] No new warnings
```

### Review Process

- Maintainers will review your PR within 3-5 business days
- Address feedback promptly
- Keep the PR focused (one feature/fix per PR)
- Rebase on `main` if conflicts arise

---

## Documentation

### Documentation Types

1. **Code Documentation**: GoDoc comments for exported APIs
2. **User Documentation**: Guides in `docs/` directory
3. **API Documentation**: OpenAPI specs for HTTP API
4. **Examples**: Code samples in `examples/` directory

### Documentation Standards

- Use clear, concise language
- Include code examples
- Update docs alongside code changes
- Test all commands and examples

### Building Documentation

Documentation is written in Markdown and hosted at [docs.volantvm.com](https://docs.volantvm.com).

```bash
# Preview documentation locally (if mkdocs is used)
mkdocs serve

# Build documentation
mkdocs build
```

---

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Discord**: Real-time chat (link TBD)
- **Email**: security@volantvm.com (security issues only)

### Getting Help

- Check [documentation](https://docs.volantvm.com)
- Search [existing issues](https://github.com/volantvm/volant/issues)
- Ask in GitHub Discussions
- Join Discord community

### Security Issues

**Do not** open public issues for security vulnerabilities. Instead:

1. Email security@volantvm.com with details
2. Include steps to reproduce
3. Allow time for fix before disclosure
4. Coordinate disclosure timeline with maintainers

We follow responsible disclosure practices and will credit reporters in security advisories.

---

## License

By contributing to Volant, you agree that your contributions will be licensed under the [Business Source License 1.1](https://github.com/volantvm/volant/blob/main/LICENSE).

The license allows:
- **Non-production use** (development, testing, evaluation)
- **Plugin development** without license restrictions
- **Commercial use** after the Change Date (2029-10-04) or with a commercial license

---

## Recognition

Contributors will be recognized in:
- `CONTRIBUTORS.md` file
- Release notes for significant contributions
- Blog posts for major features
- Conference talks and presentations

---

## Questions?

If you have questions about contributing:

- Open a [GitHub Discussion](https://github.com/volantvm/volant/discussions)
- Review existing documentation
- Reach out to maintainers

Thank you for making Volant better!

---

## Next Steps

- **[Building from Source](2_building-from-source.md)** – Detailed build instructions
- **[Architecture Overview](../5_architecture/1_overview.md)** – System architecture
- **[CLI Reference](../6_reference/3_cli-reference.md)** – Command-line interface
