# Open API and MCP

## Authentication

Planexus supports two API authentication modes:

1. Browser session cookie. `POST /api/v1/auth/login` returns a `csrfToken`; send it as `X-CSRF-Token` on POST, PUT and DELETE requests.
2. Personal Bearer key. Create it in Profile → Password & API Keys, store the one-time secret, and send `Authorization: Bearer plx_…`. The key is limited by the owner's current authorization, issued scopes, expiry, revocation state and administrator key policy.

All resource handlers apply RBAC plus classification/organization ABAC. A Bearer key does not bypass its owner context.

The live OpenAPI 3.0 document is served at `/openapi.yaml`; its version is injected from the running build.

## MCP endpoint

MCP uses authenticated JSON-RPC over Streamable HTTP at `/mcp`. An administrator can disable the server or individual Tools in Administration → MCP & API. The same RBAC/ABAC checks used by REST are applied inside each Tool.

Initialize:

```bash
curl -sS http://planexus.internal:8080/mcp \
  -H 'Authorization: Bearer plx_REDACTED' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"internal-agent","version":"1.0"}}}'
```

List the administrator-enabled Tools:

```bash
curl -sS http://planexus.internal:8080/mcp \
  -H 'Authorization: Bearer plx_REDACTED' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

Example Tool call:

```bash
curl -sS http://planexus.internal:8080/mcp \
  -H 'Authorization: Bearer plx_REDACTED' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_project_risk","arguments":{}}}'
```

Implemented Tools:

- `get_strategy`, `search_strategy`
- `get_kpi`, `get_kpi_performance`
- `get_projects`, `get_project_risk`, `get_budget_status`
- `get_decision_history`, `search_intelligence`
- `run_scenario`, `generate_executive_brief`

MCP Tool calls are written to the immutable audit log with actor, Tool name, outcome and arguments. Sensitive tokens are never persisted in the audit details.
