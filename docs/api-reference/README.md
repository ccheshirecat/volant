# API Reference

Generate OpenAPI JSON with:

```bash
make openapi-export
```

This builds `bin/openapi-export` and writes `docs/api-reference/openapi.json` with the server URL set to `https://docs.volantvm.com`.

The source of truth for the API is in `internal/server/httpapi` and the spec builder used by `cmd/openapi-export`.
