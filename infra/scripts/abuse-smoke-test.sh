#!/usr/bin/env bash
# Verify abuse auto-stop infrastructure on Back (env, DB, internal API, worker).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT/infra/docker/docker-compose.back.yml}"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "OK: $*"; }

[[ -f "$ENV_FILE" ]] || fail "missing $ENV_FILE"

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

[[ -n "${POSTGRES_DSN:-}" ]] || fail "POSTGRES_DSN not set in $ENV_FILE"
command -v psql >/dev/null 2>&1 || fail "psql not found (apt install postgresql-client)"

echo "=== abuse smoke test ==="

[[ "${ABUSE_ENABLED:-true}" == "true" ]] || fail "ABUSE_ENABLED must be true in .env"
ok "ABUSE_ENABLED=true in .env"

if [[ -z "${ABUSE_INGEST_TOKEN:-}" ]]; then
  if [[ -n "${NOTIFICATION_SERVICE_TOKEN:-}" ]]; then
    ok "ABUSE_INGEST_TOKEN empty but NOTIFICATION_SERVICE_TOKEN set (ingest auth fallback)"
    INGEST_TOKEN="$NOTIFICATION_SERVICE_TOKEN"
  else
    fail "set ABUSE_INGEST_TOKEN or NOTIFICATION_SERVICE_TOKEN in .env"
  fi
else
  INGEST_TOKEN="$ABUSE_INGEST_TOKEN"
  ok "ABUSE_INGEST_TOKEN configured (${#INGEST_TOKEN} chars)"
fi

docker ps --format '{{.Names}}' | grep -q '^docker-vps-1$' || fail "docker-vps-1 not running"
docker ps --format '{{.Names}}' | grep -q '^docker-vps-worker-1$' || fail "docker-vps-worker-1 not running"

container_abuse="$(docker exec docker-vps-1 printenv ABUSE_ENABLED 2>/dev/null || true)"
[[ "$container_abuse" == "true" ]] || fail "docker-vps-1 ABUSE_ENABLED=$container_abuse"
ok "vps container ABUSE_ENABLED=true"

worker_abuse="$(docker exec docker-vps-worker-1 printenv ABUSE_ENABLED 2>/dev/null || true)"
[[ "$worker_abuse" == "true" ]] || fail "docker-vps-worker-1 ABUSE_ENABLED=$worker_abuse"
ok "vps-worker container ABUSE_ENABLED=true"

worker_log="$(docker logs docker-vps-worker-1 2>&1 || true)"
if ! grep -Fq 'abuse detection enabled' <<< "$worker_log"; then
  fail "worker logs missing abuse detection enabled — restart vps-worker"
fi
ok "worker logs show abuse detection enabled"

psql "$POSTGRES_DSN" -tAc "SELECT to_regclass('vps.abuse_signals')" | grep -q abuse_signals \
  || fail "table vps.abuse_signals missing — run infra/scripts/migrate.sh"
ok "DB table vps.abuse_signals exists"

psql "$POSTGRES_DSN" -tAc \
  "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='vps' AND table_name='instances' AND column_name='abuse_hold')" \
  | grep -q t || fail "column vps.instances.abuse_hold missing"
ok "DB column instances.abuse_hold exists"

if psql "$POSTGRES_DSN" -tAc \
  "SELECT EXISTS(SELECT 1 FROM platform.schema_migrations WHERE service='vps' AND filename='035_abuse_detection.sql')" \
  | grep -q t; then
  ok "migration 035_abuse_detection.sql applied"
else
  echo "WARN: migration 035 not in schema_migrations (tables may exist from Go migrate on startup)"
fi

# Auth checks via curl on docker network (Alpine/busybox wget lacks --header).
NET="$(docker inspect docker-vps-1 --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}' | head -1)"
[[ -n "$NET" ]] || fail "could not detect docker network for docker-vps-1"

bad_code="$(docker run --rm --network "$NET" curlimages/curl:8.5.0 -s -o /dev/null -w '%{http_code}' -X POST \
  -H 'Content-Type: application/json' -H 'X-Service-Token: bad-token' \
  -d '{"ip":"203.0.113.1","signal_type":"provider_complaint"}' \
  http://vps:8003/internal/abuse/signal 2>/dev/null || echo 000)"
[[ "$bad_code" == "401" ]] || fail "expected 401 with bad token, got HTTP $bad_code"
ok "internal API rejects bad token (401)"

good_code="$(docker run --rm --network "$NET" curlimages/curl:8.5.0 -s -o /dev/null -w '%{http_code}' -X POST \
  -H "Content-Type: application/json" -H "X-Service-Token: ${INGEST_TOKEN}" \
  -d '{"ip":"203.0.113.1","signal_type":"provider_complaint"}' \
  http://vps:8003/internal/abuse/signal 2>/dev/null || echo 000)"
[[ "$good_code" == "404" ]] || fail "expected 404 for TEST-NET IP, got HTTP $good_code"
ok "internal API accepts token and returns 404 for unknown IP"

running="$(psql "$POSTGRES_DSN" -tAc "SELECT count(*) FROM vps.instances WHERE state='running' AND ip_address IS NOT NULL")"
with_metrics="$(psql "$POSTGRES_DSN" -tAc "SELECT count(*) FROM vps.instances WHERE state='running' AND metrics_updated_at IS NOT NULL")"
echo "INFO: running VPS with IP=$running, with metrics_updated_at=$with_metrics"
if [[ "$running" -gt 0 && "$with_metrics" -eq 0 ]]; then
  echo "WARN: no running instances have VF metrics — TX flood detector inactive until VF /metrics works"
fi

echo "=== abuse smoke test passed ==="
