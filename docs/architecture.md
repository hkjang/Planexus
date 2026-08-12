# Architecture

## Deployment unit

Planexus starts as a modular monolith. A single Go process serves the React UI, `/api/v1`, `/mcp`, `/openapi.yaml`, health probes, OIDC callbacks, background jobs, and telemetry endpoints. Domain boundaries remain explicit so high-load modules can later be extracted without changing public contracts.

The release builder is pinned to Go 1.26.5 or newer within the 1.26 line so the runtime includes all published standard-library security fixes through July 2026.

```text
Browser / AI Client
        |
  Go HTTP boundary
        |
  AuthN -> RBAC + ABAC -> Audit
        |
  Strategy | KPI | Planning | Portfolio | Decision | Intelligence | AI
        |
  PostgreSQL repositories
        |
    PostgreSQL
```

## Security decisions

- Bootstrap credentials create the first system administrator idempotently and are never stored as plaintext.
- OIDC settings are configured after bootstrap from the administration UI. Client secrets and other sensitive configuration use AES-256-GCM envelope encryption with the required `ENCRYPTION_KEY`.
- Session cookies are HttpOnly, SameSite=Lax and Secure when TLS is observed. Sessions and CSRF tokens are server-side and revocable.
- Authorization combines stable role permissions with resource attributes such as organization, owner, classification, business domain, and purpose.
- Personal API/MCP keys are shown once, stored as SHA-256 hashes, scoped, expiring, revocable, and rotatable. Rotation overlaps are policy-controlled.
- Audit rows are append-only. PostgreSQL triggers reject UPDATE and DELETE for the application role.
- Approval is a policy, not a hard-coded state machine. When disabled, submissions finalize directly; when enabled, configured review/approval/rejection steps apply.

## Configuration boundary

The application intentionally reads only four environment variables. Runtime settings live in versioned database records. Secret fields are encrypted and responses are redacted. Configuration changes are audited.

## Offline behavior

The UI has no CDN dependency. Fonts use a system stack. The production image contains the compiled UI and Go binary. External OIDC, AI, webhook, and intelligence feeds are optional; local authentication and all core strategy functions continue without network access.

The application deliberately does not bundle PostgreSQL into the service image. Air-gapped sites load `planexus:v<version>` and connect it to their managed PostgreSQL through the single required `POSTGRES_DSN`.

## Implemented module boundaries

- Identity: bootstrap/local login, Keycloak OIDC Discovery + PKCE + nonce, session/CSRF, role and organization mapping.
- Strategy core: hierarchical strategy, KPI tree fields, portfolio project risk/budget, cockpit and personal dashboard.
- Planning: plans, configurable workflow definitions, review task history and approve/reject actions.
- Decision intelligence: decisions, categorized intelligence, deterministic scenario simulation and governed AI evidence.
- Integration: Excel validation/preview/commit/rollback, notification rules, OpenAPI, MCP, encrypted connector registry.
- Operations: append-only audit and AI records, Prometheus endpoint, health, logical backup validation and transactional restore.

Global search currently uses PostgreSQL permission-filtered text matching across the core entities. A vector provider is not required for offline MVP operation; it can be added behind the search boundary without changing the public route.
