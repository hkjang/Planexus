#!/usr/bin/env sh
set -eu

VERSION=${1:-0.1.0}
IMAGE="planexus:v${VERSION}"
DB_CONTAINER=planexus-smoke-postgres
APP_CONTAINER=planexus-smoke-app
NETWORK=planexus-smoke-network
PORT=58080
DOCKER_COMMAND=${DOCKER_COMMAND:-docker}
if ! command -v "${DOCKER_COMMAND}" >/dev/null 2>&1 && command -v docker.exe >/dev/null 2>&1; then
  DOCKER_COMMAND=docker.exe
fi

cleanup() {
  "${DOCKER_COMMAND}" rm -f "${APP_CONTAINER}" "${DB_CONTAINER}" >/dev/null 2>&1 || true
  "${DOCKER_COMMAND}" network rm "${NETWORK}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
cleanup

"${DOCKER_COMMAND}" network create "${NETWORK}" >/dev/null
"${DOCKER_COMMAND}" run --rm -d --name "${DB_CONTAINER}" --network "${NETWORK}" \
  -e POSTGRES_USER=planexus \
  -e POSTGRES_PASSWORD=planexus-smoke-password \
  -e POSTGRES_DB=planexus \
  postgres:17-alpine >/dev/null

database_ready=false
for _ in $(seq 1 45); do
  if "${DOCKER_COMMAND}" exec "${DB_CONTAINER}" pg_isready -U planexus -d planexus >/dev/null 2>&1; then database_ready=true; break; fi
  sleep 1
done
if [ "${database_ready}" != true ]; then
  "${DOCKER_COMMAND}" logs "${DB_CONTAINER}" >&2
  exit 1
fi

"${DOCKER_COMMAND}" run --rm -d --name "${APP_CONTAINER}" --network "${NETWORK}" \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -p "${PORT}:8080" \
  -e POSTGRES_DSN="postgres://planexus:planexus-smoke-password@${DB_CONTAINER}:5432/planexus?sslmode=disable" \
  -e BOOTSTRAP_ADMIN=admin \
  -e BOOTSTRAP_ADMIN_PASSWORD='Planexus-Smoke-1234!' \
  -e ENCRYPTION_KEY='MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=' \
  "${IMAGE}" >/dev/null

application_ready=false
for _ in $(seq 1 45); do
  if curl -fsS "http://127.0.0.1:${PORT}/health/ready" >/dev/null 2>&1; then application_ready=true; break; fi
  sleep 1
done
if [ "${application_ready}" != true ]; then
  "${DOCKER_COMMAND}" logs "${APP_CONTAINER}" >&2 || true
  exit 1
fi

curl -fsS "http://127.0.0.1:${PORT}/health/ready" | grep -q '"status":"ready"'
curl -fsS "http://127.0.0.1:${PORT}/api/v1/version" | grep -q "\"version\":\"${VERSION}\""
curl -fsS -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Planexus-Smoke-1234!"}' \
  "http://127.0.0.1:${PORT}/api/v1/auth/login" | grep -q '"mustChangePassword":true'

test "$("${DOCKER_COMMAND}" inspect -f '{{.Config.User}}' "${APP_CONTAINER}" | tr -d '\r')" = '65532:65532'
test "$("${DOCKER_COMMAND}" inspect -f '{{.HostConfig.ReadonlyRootfs}}' "${APP_CONTAINER}" | tr -d '\r')" = 'true'
for variable_name in POSTGRES_DSN BOOTSTRAP_ADMIN BOOTSTRAP_ADMIN_PASSWORD ENCRYPTION_KEY; do
  "${DOCKER_COMMAND}" inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "${APP_CONTAINER}" | tr -d '\r' | grep -q "^${variable_name}="
done
test "$("${DOCKER_COMMAND}" inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "${APP_CONTAINER}" | tr -d '\r' | sed '/^$/d;/^PATH=/d' | wc -l)" -eq 4

echo "Planexus container smoke test passed for ${IMAGE}"
