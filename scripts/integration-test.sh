#!/usr/bin/env sh
set -eu

DB_CONTAINER=planexus-integration-postgres
DB_PORT=55432
DOCKER_COMMAND=${DOCKER_COMMAND:-docker}
if ! command -v "${DOCKER_COMMAND}" >/dev/null 2>&1 && command -v docker.exe >/dev/null 2>&1; then
  DOCKER_COMMAND=docker.exe
fi
COOKIE_FILE=/tmp/planexus-integration-cookie
LOGIN_FILE=/tmp/planexus-integration-login.json
SERVER_LOG=/tmp/planexus-integration-server.log
IMPORT_FILE=/tmp/planexus-integration-import.xlsx
BACKUP_FILE=/tmp/planexus-integration-backup.plxbackup
SERVER_PID=

cleanup() {
  if [ -n "${SERVER_PID}" ]; then kill "${SERVER_PID}" 2>/dev/null || true; fi
  "${DOCKER_COMMAND}" stop "${DB_CONTAINER}" >/dev/null 2>&1 || true
  unlink "${COOKIE_FILE}" 2>/dev/null || true
  unlink "${LOGIN_FILE}" 2>/dev/null || true
  unlink "${SERVER_LOG}" 2>/dev/null || true
  unlink "${IMPORT_FILE}" 2>/dev/null || true
  unlink "${BACKUP_FILE}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

"${DOCKER_COMMAND}" run --rm -d --name "${DB_CONTAINER}" \
  -e POSTGRES_USER=planexus \
  -e POSTGRES_PASSWORD=planexus-test-password \
  -e POSTGRES_DB=planexus \
  -p "${DB_PORT}:5432" postgres:17-alpine >/dev/null

for _ in $(seq 1 30); do
  if "${DOCKER_COMMAND}" exec "${DB_CONTAINER}" pg_isready -U planexus -d planexus >/dev/null 2>&1; then break; fi
  sleep 1
done

POSTGRES_DSN="postgres://planexus:planexus-test-password@127.0.0.1:${DB_PORT}/planexus?sslmode=disable" \
BOOTSTRAP_ADMIN=admin \
BOOTSTRAP_ADMIN_PASSWORD='Planexus-Test-1234!' \
ENCRYPTION_KEY='MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=' \
go run -ldflags '-X main.version=integration' ./cmd/planexus >"${SERVER_LOG}" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then break; fi
  sleep 1
done

curl -fsS -c "${COOKIE_FILE}" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Planexus-Test-1234!"}' \
  http://127.0.0.1:8080/api/v1/auth/login >"${LOGIN_FILE}"
CSRF=$(jq -r .csrfToken "${LOGIN_FILE}")
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"currentPassword":"Planexus-Test-1234!","newPassword":"Planexus-Test-Rotated-1234!"}' \
  http://127.0.0.1:8080/api/v1/auth/password >/dev/null

ORGANIZATION_ID=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"code":"HQ","name":"Corporate Strategy","attributes":{"businessDomain":"enterprise"}}' \
  http://127.0.0.1:8080/api/v1/admin/organizations | jq -r .id)
ADMIN_ID=$(curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/admin/users | jq -er 'map(select(.username == "admin"))[0].id')
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d "{\"displayName\":\"Planexus Administrator\",\"email\":\"admin@planexus.local\",\"title\":\"System Administrator\",\"organizationId\":\"${ORGANIZATION_ID}\",\"active\":true}" \
  "http://127.0.0.1:8080/api/v1/admin/users/${ADMIN_ID}" >/dev/null
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/admin/users | \
  jq -e --arg organization_id "${ORGANIZATION_ID}" 'map(select(.username == "admin"))[0].organizationId == $organization_id' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"name":"General User","description":"Authorized business user","permissions":["dashboard:read","strategy:read","kpi:read"]}' \
  http://127.0.0.1:8080/api/v1/admin/roles/general_user >/dev/null
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/admin/roles | \
  jq -e 'map(select(.id == "general_user"))[0].permissions | index("strategy:read") != null' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"sensitive":false,"value":{"timeoutMinutes":120}}' \
  http://127.0.0.1:8080/api/v1/admin/settings/system/session >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"sensitive":false,"value":{"requestsPerMinute":600}}' \
  http://127.0.0.1:8080/api/v1/admin/settings/system/api_rate_limit >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"sensitive":false,"value":{"maxLifetimeDays":30,"rotationOverlapHours":12,"allowedScopes":["strategy:read","kpi:read","project:read","decision:read","dashboard:read"]}}' \
  http://127.0.0.1:8080/api/v1/admin/settings/security/personal_keys >/dev/null
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/keys/policy | \
  jq -e '.maxLifetimeDays == 30 and .rotationOverlapHours == 12 and (.allowedScopes | index("strategy:read") != null)' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"sensitive":true,"value":{"credential":"integration-secret"}}' \
  http://127.0.0.1:8080/api/v1/admin/settings/integration_credentials/test_rest >/dev/null
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/admin/settings | \
  jq -e 'map(select(.category == "integration_credentials" and .key == "test_rest"))[0] | .sensitive == true and has("value") == false' >/dev/null

STRATEGY_ID=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"name":"Integration Strategy","kind":"objective"}' http://127.0.0.1:8080/api/v1/strategies | jq -r .id)
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d "{\"strategyId\":\"${STRATEGY_ID}\",\"code\":\"IT-001\",\"name\":\"Integration KPI\",\"target\":100,\"actual\":82}" \
  http://127.0.0.1:8080/api/v1/kpis >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d "{\"strategyId\":\"${STRATEGY_ID}\",\"name\":\"Integration Project\",\"risk\":\"high\",\"budget\":1000000,\"actualCost\":420000}" \
  http://127.0.0.1:8080/api/v1/projects >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"category":"competitor","title":"Competitor AI investment","summary":"Investment increased","importance":85}' \
  http://127.0.0.1:8080/api/v1/intelligence >/dev/null
curl -fsS -b "${COOKIE_FILE}" 'http://127.0.0.1:8080/api/v1/search?q=Integration' | \
  jq -e 'map(.type) | index("strategy") != null and index("kpi") != null and index("project") != null' >/dev/null
SCENARIO_ID=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"name":"Cost reduction","assumptions":{"costReductionPercent":10}}' \
  http://127.0.0.1:8080/api/v1/scenarios | jq -r .id)
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -d '{}' \
  "http://127.0.0.1:8080/api/v1/scenarios/${SCENARIO_ID}/run" | jq -e '.simulatedBudget == 900000' >/dev/null
PLAN_ID=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"title":"Integration Plan","period":"2027","content":{}}' http://127.0.0.1:8080/api/v1/plans | jq -r .id)
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -d '{}' \
  "http://127.0.0.1:8080/api/v1/plans/${PLAN_ID}/submit" | jq -e '.status == "confirmed"' >/dev/null

KEY=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"name":"Integration MCP","scopes":["strategy:read","kpi:read","dashboard:read"],"expiresInDays":1}' \
  http://127.0.0.1:8080/api/v1/keys | jq -r .token)
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/dashboard/personal | \
  jq -e '.activeKeys == 1 and (.recommendations | map(.type) | index("key") != null)' >/dev/null
curl -fsS -H "Authorization: Bearer ${KEY}" http://127.0.0.1:8080/api/v1/dashboard/personal | jq -e '.activeKeys == 1' >/dev/null
test "$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${KEY}" http://127.0.0.1:8080/api/v1/keys)" = 403
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"sensitive":false,"value":{"enabled":true,"allowedTools":["generate_executive_brief"]}}' \
  http://127.0.0.1:8080/api/v1/admin/settings/integration/mcp >/dev/null
curl -fsS -H "Authorization: Bearer ${KEY}" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  http://127.0.0.1:8080/mcp | jq -e '.result.tools | length == 1 and .[0].name == "generate_executive_brief"' >/dev/null
curl -fsS -H "Authorization: Bearer ${KEY}" -H 'Content-Type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"get_strategy\",\"arguments\":{\"id\":\"${STRATEGY_ID}\"}}}" \
  http://127.0.0.1:8080/mcp | jq -e '.error.code == -32003' >/dev/null
curl -fsS -H "Authorization: Bearer ${KEY}" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"generate_executive_brief","arguments":{}}}' \
  http://127.0.0.1:8080/mcp | jq -e '.result.isError == false' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"query":"현재 KPI와 프로젝트 위험을 알려줘","useCase":"strategy"}' \
  http://127.0.0.1:8080/api/v1/ai/query | jq -e '.confidence == 1 and .model == "planexus-deterministic"' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"name":"KPI below target","entityType":"kpi","conditionField":"achievement","operator":"lt","threshold":"90","severity":"critical","channels":["system"],"global":true,"cooldownMinutes":60}' \
  http://127.0.0.1:8080/api/v1/notification-rules >/dev/null
curl -fsS -b "${COOKIE_FILE}" 'http://127.0.0.1:8080/api/v1/notifications?unread=true' | jq -e 'length >= 1 and .[0].severity == "critical"' >/dev/null
curl -fsS -b "${COOKIE_FILE}" 'http://127.0.0.1:8080/api/v1/import/templates/strategy?sample=true' >"${IMPORT_FILE}"
IMPORT_ID=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" \
  -F entityType=strategy -F "file=@${IMPORT_FILE}" http://127.0.0.1:8080/api/v1/import/preview | jq -er 'select(.validRows == 1 and .invalidRows == 0) | .id')
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -d '{}' \
  "http://127.0.0.1:8080/api/v1/import/${IMPORT_ID}/commit" | jq -e '.importedRows == 1' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -d '{}' \
  "http://127.0.0.1:8080/api/v1/import/${IMPORT_ID}/rollback" | jq -e '.removedRows == 1' >/dev/null
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -X PUT \
  -d '{"enabled":true,"steps":[{"type":"approval","order":0}],"rejectionPolicy":"return_to_author"}' \
  http://127.0.0.1:8080/api/v1/admin/workflows/plan >/dev/null
REVIEW_PLAN_ID=$(curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"title":"Approval Plan","period":"2028","content":{}}' http://127.0.0.1:8080/api/v1/plans | jq -r .id)
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' -d '{}' \
  "http://127.0.0.1:8080/api/v1/plans/${REVIEW_PLAN_ID}/submit" | jq -e '.status == "in_review"' >/dev/null
TASK_ID=$(curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/workflow/tasks | jq -er '.[0].id')
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -H 'Content-Type: application/json' \
  -d '{"action":"approve","comment":"approved by integration test"}' "http://127.0.0.1:8080/api/v1/workflow/tasks/${TASK_ID}/action" | jq -e '.status == "approved" and .planStatus == "confirmed"' >/dev/null
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/admin/backups/export >"${BACKUP_FILE}"
curl -fsS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -F "file=@${BACKUP_FILE}" \
  http://127.0.0.1:8080/api/v1/admin/backups/validate | jq -e '.valid == true' >/dev/null
curl -sS -b "${COOKIE_FILE}" -H "X-CSRF-Token: ${CSRF}" -F "file=@${BACKUP_FILE}" -F 'confirmation=RESTORE PLANEXUS' \
  http://127.0.0.1:8080/api/v1/admin/backups/restore | jq -e 'if .status == "restored" and .loginRequired == true then true else error(tostring) end' >/dev/null
curl -fsS -c "${COOKIE_FILE}" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Planexus-Test-Rotated-1234!"}' http://127.0.0.1:8080/api/v1/auth/login >"${LOGIN_FILE}"
curl -fsS -b "${COOKIE_FILE}" http://127.0.0.1:8080/api/v1/strategies | jq -e 'length >= 1' >/dev/null
curl -fsS http://127.0.0.1:8080/openapi.yaml | grep -q 'version: integration'
curl -fsS http://127.0.0.1:8080/metrics | grep -q 'planexus_http_requests_total'
SESSION_MINUTES=$("${DOCKER_COMMAND}" exec "${DB_CONTAINER}" psql -U planexus -d planexus -tAc \
  "SELECT round(EXTRACT(epoch FROM (expires_at-created_at))/60) FROM sessions ORDER BY created_at DESC LIMIT 1" | tr -d '\r ')
test "${SESSION_MINUTES}" = 120

echo "Planexus integration test passed"
