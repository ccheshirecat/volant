# API Access Control

## Objectives
- Provide lightweight protection for the local REST API during development.
- Support CIDR-based allow lists and API keys until a production authn/z story is implemented.
- Establish patterns that can evolve into mTLS, OIDC, or token-based auth without reworking handlers.

## Mechanisms
1. **CIDR Filtering (`VOLANT_API_ALLOW_CIDR`)**
   - Comma-separated list of CIDR blocks evaluated against the client IP.
   - Default: empty (accept all).
   - IPv4 supported. You can extend to IPv6 in future versions.
2. **API Token (`VOLANT_API_KEY`)**
   - Provide shared secret via header `X-Volant-API-Key: <token>` or query `?api_key=<token>`.
   - Configure the server environment with `VOLANT_API_KEY` to enable.
   - Store tokens in secret stores for production and inject via systemd drop-ins or container env.

## Operational Notes
- Terminate TLS in front of volantd (nginx, Caddy, Envoy) for transport security.
- Prefer header-based tokens over query params when possible (query params may be logged by proxies).
- Rotate tokens regularly; use systemd `EnvironmentFile=` to avoid storing secrets in unit files.

## Gaps & Next Steps
- Rate limiting and brute-force detection.
- Centralized config struct and hot-reload.
- Structured audit logging for allow/deny events.

## Future Enhancements
- Pluggable auth providers (API key, JWT, OIDC).
- Role-based authorization tied to CLI/MCP identities.
- Mutual TLS between CLI/agents and server.
