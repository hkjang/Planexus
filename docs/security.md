# Security model

## Trust boundaries

- The reverse proxy terminates approved internal TLS. Planexus uses the TCP peer for audit IP and does not trust arbitrary `X-Forwarded-For` values.
- PostgreSQL is the authoritative store for sessions, roles, permissions, configuration versions, audit and domain data.
- `ENCRYPTION_KEY` is a required, base64-encoded 32-byte root key. Planexus uses AES-256-GCM with contextual AAD for OIDC secrets, AI model credentials, connector credentials and AI prompt/response audit payloads.
- Changing `ENCRYPTION_KEY` without re-encrypting data makes encrypted settings unreadable. Back it up separately and restrict access.

## Identity and authorization

- The bootstrap administrator is created idempotently with bcrypt and is forced to change the initial password.
- Local login is rate limited. Session cookies are HttpOnly and SameSite=Lax, become Secure behind HTTPS, and unsafe browser methods require a server-side CSRF token.
- Authenticated REST and MCP traffic uses an administrator-configurable per-user request limit; over-limit responses include HTTP 429 and `Retry-After`.
- Keycloak/OIDC uses Discovery, authorization code flow, PKCE S256, encrypted state, expiry and nonce verification. Issuer and audience are verified by the OIDC verifier. Group and realm-role mappings synchronize configured Planexus roles.
- RBAC permissions support exact actions and `resource:*` wildcards. ABAC additionally checks data classification and organization context. `Executive` and `Restricted` records receive stricter checks.
- System administrators can edit role permissions, but cannot remove the `system_admin` wildcard from the built-in administrator role or disable/remove their own active administrator access through the API.

## Personal API and MCP keys

- A key secret is generated with cryptographic randomness, displayed once and stored only as SHA-256.
- Issued scopes cannot exceed the user's authorization or the administrator's allowed-scope policy. Effective authorization is recalculated on every request as the intersection of stored key scopes and current role permissions, so later role revocation takes effect immediately.
- Keys expire, can be revoked, and update last-used metadata. Rotation creates a replacement and shortens the old key to the configured overlap window.
- Key issuance, rotation, listing and revocation require an interactive browser session; one personal key cannot administer the user's other keys.
- MCP applies both the personal key scopes and the current user's organization/classification context. Administrators can disable MCP or remove individual Tools from exposure.

## AI guardrails

- Every model call passes through the internal AI Gateway.
- Administrator policy controls external-provider use, use-case routing, model priority, timeout, token limits, PII masking and prompt-injection blocking. PII masking covers both the user's question and serialized evidence sent to a model.
- Evidence is selected only from data authorized for the caller. Responses include answer, confidence, evidence, source, generation time and model.
- AI prompts and responses are encrypted at rest; interaction metadata and immutable audit records retain accountability.

## Deployment hardening

- Run the scratch image as its built-in non-root user with a read-only root filesystem and a small `noexec,nosuid` `/tmp` tmpfs.
- Terminate TLS at an approved reverse proxy, enforce PostgreSQL TLS in `POSTGRES_DSN`, and restrict database/network access to required peers.
- Treat `/metrics`, health and OpenAPI exposure according to internal network policy.
- Scan uploaded Excel content and future attachments at the ingress boundary. The current MVP accepts only bounded XLSX imports and does not expose a general attachment service.
