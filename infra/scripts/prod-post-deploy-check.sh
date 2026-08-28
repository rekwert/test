#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker && set -a && source .env && set +a
echo "=== GIT ==="
git -C /opt/testVPStrade rev-parse --short HEAD
echo "=== HEALTH ==="
curl -sf http://127.0.0.1:8080/health
echo
echo "=== OUTBOX unpublished ==="
psql "$POSTGRES_DSN" -c "SELECT event_type, count(*) FROM vps.outbox WHERE published = false GROUP BY event_type ORDER BY 2 DESC;"
echo "=== INSTANCES by state ==="
psql "$POSTGRES_DSN" -c "SELECT state, count(*) FROM vps.instances GROUP BY state ORDER BY 2 DESC;"
echo "=== QUEUED instances ==="
psql "$POSTGRES_DSN" -c "SELECT id, region, state FROM vps.instances WHERE state='queued' LIMIT 5;"
echo "=== ENCRYPTION ==="
psql "$POSTGRES_DSN" -c "SELECT count(*) FILTER (WHERE root_password LIKE 'enc:%') AS enc_pw, count(*) FILTER (WHERE root_password IS NOT NULL AND root_password NOT LIKE 'enc:%') AS plain_pw FROM vps.instances;"
psql "$POSTGRES_DSN" -c "SELECT count(*) FILTER (WHERE ed25519_private_key LIKE 'enc:%') AS enc_ed, count(*) FILTER (WHERE ed25519_private_key IS NOT NULL AND ed25519_private_key NOT LIKE 'enc:%') AS plain_ed FROM vps.ip_ssh_host_keys;"
echo "=== TRIAL billing_period_days ==="
psql "$POSTGRES_DSN" -c "SELECT id, billing_period_days, auto_renew, next_billing_at::date FROM vps.instances WHERE trial_free_week = true AND state != 'deleted' LIMIT 5;"
echo "=== WORKER tail ==="
docker logs docker-vps-worker-1 --tail 8 2>&1
