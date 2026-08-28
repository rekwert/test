#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a
source .env
set +a

echo "=== GIT HEAD ==="
git -C /opt/testVPStrade rev-parse --short HEAD
git -C /opt/testVPStrade log -1 --oneline

echo "=== ENCRYPTION KEY ==="
if grep -q '^VPS_FIELD_ENCRYPTION_KEY=' /opt/testVPStrade/infra/docker/.env; then
  echo "VPS_FIELD_ENCRYPTION_KEY=set"
else
  echo "VPS_FIELD_ENCRYPTION_KEY=MISSING"
fi

echo "=== DOCKER STATUS ==="
docker ps --format 'table {{.Names}}\t{{.Status}}'

echo "=== HEALTH ==="
curl -sf "http://127.0.0.1:${GATEWAY_PORT:-8080}/health"; echo
curl -sf "http://127.0.0.1:${GATEWAY_PORT:-8080}/api/v1/health"; echo

echo "=== GATEWAY AUDIT ==="
bash /opt/testVPStrade/infra/scripts/audit-gateway-routes.sh 2>&1 | tail -8

echo "=== INSTANCE STATES ==="
psql "$POSTGRES_DSN" -c "SELECT state, count(*) FROM vps.instances WHERE state <> 'deleted' GROUP BY state ORDER BY 1;"
psql "$POSTGRES_DSN" -c "SELECT o.order_number, i.state, i.region, host(i.ip_address) ip, i.provider_meta->>'wait_reason' wr FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE i.state IN ('creating','queued','error') ORDER BY o.order_number;"

echo "=== SCHEMA MIGRATIONS ==="
psql "$POSTGRES_DSN" -c "SELECT service, version FROM platform.schema_migrations ORDER BY service;"

echo "=== WORKER last 40 lines ==="
docker logs docker-vps-worker-1 --tail 40 2>&1

echo "=== WORKER errors last 20m ==="
docker logs docker-vps-worker-1 --since 20m 2>&1 | grep -iE 'error|fail|panic' | grep -v 'hostkey sync' | tail -20 || echo none

echo "=== BILLING WORKER ==="
docker logs docker-billing-worker-1 --tail 15 2>&1

echo "=== NOTIFICATION WORKER ==="
docker logs docker-notification-worker-1 --tail 15 2>&1

echo "=== ENCRYPTED PASSWORDS SAMPLE ==="
psql "$POSTGRES_DSN" -c "SELECT count(*) FILTER (WHERE root_password LIKE 'enc:v1:%') enc, count(*) FILTER (WHERE root_password IS NOT NULL AND root_password <> '' AND root_password NOT LIKE 'enc:v1:%') plain FROM vps.instances;"
