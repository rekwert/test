#!/usr/bin/env bash
set -euo pipefail
echo "=== VPS WORKER errors 12h ==="
docker logs docker-vps-worker-1 --since 12h 2>&1 | grep -iE 'error|fail|panic|fatal' | grep -v 'hostkey sync' | grep -v 'dedicated sync' | tail -30 || echo "(none)"
echo "=== BILLING WORKER errors 12h ==="
docker logs docker-billing-worker-1 --since 12h 2>&1 | grep -iE 'error|fail|panic|fatal' | tail -15 || echo "(none)"
echo "=== NOTIFICATION errors 12h ==="
docker logs docker-notification-worker-1 --since 12h 2>&1 | grep -iE 'error|fail|panic|fatal' | tail -15 || echo "(none)"
echo "=== GATEWAY errors 12h ==="
docker logs docker-gateway-1 --since 12h 2>&1 | grep -iE 'error|fail|panic' | tail -10 || echo "(none)"
echo "=== VPS API errors 12h ==="
docker logs docker-vps-1 --since 12h 2>&1 | grep -iE 'error|fail|panic' | tail -10 || echo "(none)"
echo "=== OUTBOX pending ==="
cd /opt/testVPStrade/infra/docker && set -a && source .env && set +a
psql "$POSTGRES_DSN" -c "SELECT event_type, count(*) FROM vps.outbox WHERE published_at IS NULL GROUP BY event_type;"
echo "=== RECENT PROVISION/DESTROY ==="
docker logs docker-vps-worker-1 --since 12h 2>&1 | grep -iE 'provision|destroy|waitlist|requeue|complete' | tail -20 || echo "(none)"
