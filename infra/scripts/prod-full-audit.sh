#!/usr/bin/env bash
set -euo pipefail

echo "========== BACK $(hostname) $(date -u +%Y-%m-%dT%H:%M:%SZ) =========="
echo "=== GIT testVPStrade ==="
git -C /opt/testVPStrade fetch origin -q 2>/dev/null || true
LOCAL=$(git -C /opt/testVPStrade rev-parse --short HEAD 2>/dev/null || echo none)
REMOTE=$(git -C /opt/testVPStrade rev-parse --short origin/main 2>/dev/null || echo none)
echo "local=$LOCAL origin/main=$REMOTE"
git -C /opt/testVPStrade log -1 --oneline 2>/dev/null || true
if [[ "$LOCAL" != "$REMOTE" ]]; then
  echo "WARN: back repo out of sync with origin"
fi

echo "=== GIT FrontVPS ==="
git -C /opt/FrontVPS fetch origin -q 2>/dev/null || true
FL=$(git -C /opt/FrontVPS rev-parse --short HEAD 2>/dev/null || echo none)
FR=$(git -C /opt/FrontVPS rev-parse --short origin/main 2>/dev/null || echo none)
echo "local=$FL origin/main=$FR"
git -C /opt/FrontVPS log -1 --oneline 2>/dev/null || true

echo "=== DOCKER ==="
docker ps --format 'table {{.Names}}\t{{.Status}}' 2>/dev/null | head -15

echo "=== HEALTH ==="
curl -sf http://127.0.0.1:8080/health 2>/dev/null && echo || echo "gateway health FAIL"
curl -sf http://127.0.0.1:8080/api/v1/health 2>/dev/null && echo || echo "api health FAIL"

echo "=== GATEWAY AUDIT ==="
bash /opt/testVPStrade/infra/scripts/audit-gateway-routes.sh 2>&1 | tail -6

cd /opt/testVPStrade/infra/docker
set -a
source .env
set +a

echo "=== INSTANCES ==="
psql "$POSTGRES_DSN" -c "SELECT state, count(*) FROM vps.instances WHERE state <> 'deleted' GROUP BY state ORDER BY 1;"
psql "$POSTGRES_DSN" -c "SELECT o.order_number, i.state, i.region, host(i.ip_address) ip, i.provider_meta->>'wait_reason' wr FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE i.state IN ('creating','queued','error','reinstalling') ORDER BY o.order_number;"

echo "=== MIGRATIONS vps ==="
psql "$POSTGRES_DSN" -c "SELECT MAX(version) FROM platform.schema_migrations WHERE service='vps';"

echo "=== WORKER ERRORS last 12h ==="
docker logs docker-vps-worker-1 --since 12h 2>&1 | grep -iE 'error|fail|panic|fatal' | grep -v 'hostkey sync' | grep -v 'dedicated sync' | tail -30 || echo "(none)"

echo "=== WORKER recent (last 25 lines) ==="
docker logs docker-vps-worker-1 --tail 25 2>&1

echo "=== BILLING WORKER errors 12h ==="
docker logs docker-billing-worker-1 --since 12h 2>&1 | grep -iE 'error|fail|panic|fatal' | tail -15 || echo "(none)"

echo "=== NOTIFICATION WORKER errors 12h ==="
docker logs docker-notification-worker-1 --since 12h 2>&1 | grep -iE 'error|fail|panic|fatal|smtp' | tail -15 || echo "(none)"

echo "=== GATEWAY errors 12h ==="
docker logs docker-gateway-1 --since 12h 2>&1 | grep -iE 'error|fail|panic' | tail -10 || echo "(none)"

echo "=== VPS API errors 12h ==="
docker logs docker-vps-1 --since 12h 2>&1 | grep -iE 'error|fail|panic' | tail -10 || echo "(none)"

echo "=== ENCRYPTION KEY ==="
grep -q '^VPS_FIELD_ENCRYPTION_KEY=.\+' /opt/testVPStrade/infra/docker/.env && echo "VPS_FIELD_ENCRYPTION_KEY=set" || echo "VPS_FIELD_ENCRYPTION_KEY=MISSING"

echo "=== DONE BACK ==="
