#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a
source .env
set +a
psql "$POSTGRES_DSN" -c "SELECT count(*) FILTER (WHERE root_password LIKE 'enc:v1:%') AS encrypted, count(*) FILTER (WHERE root_password IS NOT NULL AND root_password <> '' AND root_password NOT LIKE 'enc:v1:%') AS plaintext FROM vps.instances;"
psql "$POSTGRES_DSN" -c "SELECT count(*) FILTER (WHERE ed25519_private LIKE 'enc:v1:%') AS enc_keys, count(*) FILTER (WHERE ed25519_private IS NOT NULL AND ed25519_private <> '' AND ed25519_private NOT LIKE 'enc:v1:%') AS plain_keys FROM vps.ip_ssh_host_keys;"
docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE 'legacy secret|reencrypt|waitlist promoted|error|fail' | tail -15 || echo "no notable worker events"
