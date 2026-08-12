# Operations

## First start

1. Create a PostgreSQL database and application account.
2. Provide the four required environment variables.
3. Start the `planexus:v<version>` image.
4. Sign in with `BOOTSTRAP_ADMIN` and immediately rotate its password in the profile/security page.
5. Configure OIDC and policy under Administration.

Schema migrations and bootstrap creation run transactionally at startup. Readiness becomes healthy after both finish.

## Container example

```bash
docker load < planexus-v0.1.0.tar.gz
docker run -d --name planexus --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -p 8080:8080 \
  -e POSTGRES_DSN='postgres://planexus:password@db:5432/planexus?sslmode=require' \
  -e BOOTSTRAP_ADMIN='admin' \
  -e BOOTSTRAP_ADMIN_PASSWORD='replace-this' \
  -e ENCRYPTION_KEY='base64-32-byte-key' \
  planexus:v0.1.0
```

Terminate TLS at an approved internal reverse proxy and forward `X-Forwarded-Proto: https`. Planexus deliberately records the TCP peer address unless a future trusted-proxy policy is configured; it does not trust arbitrary client-supplied `X-Forwarded-For` headers.

## Probes

- Liveness: `GET /health/live`
- Readiness: `GET /health/ready`
- Version: `GET /api/v1/version`
- Prometheus text metrics: `GET /metrics`
- OpenAPI 3.0: `GET /openapi.yaml`

## Administration

The service administrator can configure organizations, user status and roles, editable role permissions, Keycloak/OIDC, optional planning workflow, notifications, personal key lifetime/scopes, AI providers, integration registry, MCP Tool exposure, session timeout, per-user API rate limits, audit, backup and system health. Sensitive OIDC, AI and connector values are encrypted and never returned after save.

System administration is a separate navigation and route from My Workspace and Profile Security. Non-administrators are redirected from the administration route; every administrative API independently enforces `system_admin` permission.

## Backup and restore

Administration → Backup exports a complete logical `.plxbackup` archive. Format v2 includes a manifest, schema version, table snapshots, per-entry SHA-256 digests and an HMAC-SHA-256 signature derived from `ENCRYPTION_KEY`. Before restore, Planexus validates archive structure, schema compatibility, signed checksums, table set and key fingerprint.

Restore requires the exact confirmation text `RESTORE PLANEXUS`. It replaces application data transactionally and invalidates all sessions; log in again after completion. Retain the original `ENCRYPTION_KEY` separately or encrypted settings cannot be restored.

For infrastructure-level disaster recovery, also retain regular PostgreSQL physical/logical backups under the site's RPO/RTO policy. Application backups do not replace WAL/volume recovery.

## Operational notes

- The application process writes no persistent local files and supports a read-only root filesystem.
- Notification rules are evaluated at one-minute intervals. System notifications work offline; email, messenger and webhook delivery remains `not_configured` until an integration implementation is registered.
- The configured Session Timeout applies to newly created sessions and is enforced by the database expiry.
- AI remains useful offline through its deterministic evidence-based fallback. Registered local/private LLM endpoints are optional; external model use requires the explicit administrator policy.
- Audit and AI interaction rows are protected by PostgreSQL triggers against UPDATE and DELETE by the application path.
