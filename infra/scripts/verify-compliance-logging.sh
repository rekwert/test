#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

GW="${BACK_BIND_IP:-192.168.0.2}"
echo "=== GIT ==="
git -C "$ROOT" rev-parse --short HEAD
echo "=== HEALTH ($GW:8080) ==="
curl -sf "http://${GW}:8080/health"
echo
echo "=== COMPLIANCE TABLES ==="
psql "$POSTGRES_DSN" -c "
SELECT relname, n_live_tup
FROM pg_stat_user_tables
WHERE schemaname = 'auth'
  AND relname IN ('login_attempts', 'http_access_log', 'user_email_history')
ORDER BY relname;"
echo "=== AUDIT PORT COLUMNS ==="
psql "$POSTGRES_DSN" -At -c "
SELECT column_name FROM information_schema.columns
WHERE table_schema = 'auth' AND table_name = 'audit_log'
  AND column_name IN ('client_port', 'server_port')
ORDER BY 1;"
echo "=== HTTP ACCESS LOG (last 5) ==="
psql "$POSTGRES_DSN" -c "
SELECT id, method, path, status_code, server_port, created_at
FROM auth.http_access_log
ORDER BY id DESC LIMIT 5;"
echo "=== MIGRATION 014 ==="
psql "$POSTGRES_DSN" -At -c "
SELECT filename FROM platform.schema_migrations
WHERE service = 'auth' AND filename = '014_compliance_logging.sql';"
