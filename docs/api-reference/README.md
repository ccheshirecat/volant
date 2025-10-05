# API Reference

Generate OpenAPI JSON with:

```bash
make openapi-export
```

This builds `bin/openapi-export` and writes `docs/api-reference/openapi.json` with the server URL set to `https://docs.volantvm.com`.

References:
- internal/server/httpapi
- cmd/openapi-export (spec builder)
