# Contributing

Thanks for your interest in Volant! This document keeps it simple and actionable.

## Build prerequisites
- Go 1.22+
- Linux with KVM for end-to-end runtime tests
- On macOS: cross-compile with `GOOS=linux` when needed

## Build and test
```bash
make build
make test
```

Generate OpenAPI (used by docs site):
```bash
make openapi-export
```

## Code style
- `go fmt ./...`
- `go vet ./...`
- Small, focused commits with clear messages

## Pull Requests
- Reference issues and explain the rationale
- Include logs of passing builds/tests when touching code paths
- For docs-only PRs: ensure internal links are valid

## Security
Please report vulnerabilities responsibly (see Security policy).
