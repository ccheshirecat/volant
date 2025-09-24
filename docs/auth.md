---
title: Authentication
description: API access control and security
---

# API Access Control (Draft)

## Objectives
- Provide lightweight protection for the local REST API during development.
- Support CIDR-based allow lists and API keys until a production authn/z story is implemented.
- Establish patterns that can evolve into mTLS, OIDC, or token-based auth without reworking handlers.

## Current Mechanisms
1. **CIDR Filtering (`VIPER_API_ALLOW_CIDR`)**
   - Comma-separated list of CIDR blocks evaluated against the client IP.
   - Default: empty (accept all).
   - IPv4 only at present; extend to IPv6 by parsing `IP.To16()`.
2. **API Key (`VIPER_API_KEY`)**
   - Shared secret provided via `X-Viper-API-Key` header or `api_key` query parameter.
   - Keys are stored in environment variables for now; future work should integrate secret management.

## Gaps & Next Steps
- No rate limiting or brute-force detection.
- Lack of HTTPS termination; recommendation is to run behind reverse proxy for now.
- Need centralized configuration struct (instead of reading env vars inside middleware) to support hot reload.
- Consider adding structured audit logging for allow/deny events.

## Future Enhancements (v2.1+)
- Pluggable auth providers (API key, JWT, OIDC).
- Role-based authorization tied to CLI/MCP identities.
- Mutual TLS between CLI/agents and server.
