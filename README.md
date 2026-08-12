# Planexus

Planexus is an offline-ready Enterprise Strategy & Planning Intelligence Platform. It connects strategy, KPI, plans, portfolios, performance, decisions, and governed AI in one operating system.

The current release is a production-shaped MVP: strategy/KPI/project portfolio, business plans with optional approval, decision and intelligence records, deterministic scenarios, governed AI, permission-filtered global search, Excel import/rollback, notification rules, immutable audit, backup/restore, OpenAPI and MCP are executable end to end.

## Technology

- Go modular monolith, REST/OpenAPI and MCP Streamable HTTP
- React + TypeScript + Material UI
- PostgreSQL (JSONB, full-text search, optional pgvector)
- Local bootstrap administrator and optional Keycloak/OIDC SSO
- One self-contained application image for air-gapped networks

Material UI was selected for its mature accessibility primitives, dense enterprise components, predictable theming, and long-term React support. Planexus applies a custom design system with a 16px body baseline, visible focus states, and responsive layouts.

## Required environment

Only these variables are read at runtime:

```text
POSTGRES_DSN
BOOTSTRAP_ADMIN
BOOTSTRAP_ADMIN_PASSWORD
ENCRYPTION_KEY
```

`ENCRYPTION_KEY` must be a base64-encoded 32-byte value. Generate one on a connected administrative workstation:

```bash
openssl rand -base64 32
```

All other settings—including OIDC issuer/client credentials, role/group mappings, workflow, API/MCP policies, AI providers, integrations, notifications, and security policy—are managed in Administration and encrypted where sensitive.

## Development

```bash
make frontend
go run ./cmd/planexus
```

The server listens on `:8080`. The bootstrap password must be changed on first login. Core development checks are:

```bash
go test ./...
go vet ./...
./scripts/integration-test.sh
```

Documentation:

- [Architecture](docs/architecture.md)
- [Operations and backup](docs/operations.md)
- [Open API and MCP](docs/api-and-mcp.md)
- [Security model](docs/security.md)
- [Offline release procedure](docs/release.md)

## Offline release image

```bash
make release-archive VERSION=0.1.0
```

This creates image `planexus:v0.1.0` and `dist/planexus-v0.1.0.tar.gz`. No PostgreSQL image is bundled because `POSTGRES_DSN` points to the enterprise PostgreSQL service. The release workflow attaches only the compressed service image to GitHub Releases.
