#!/usr/bin/env bash
# Zero-surprise DB host cutover: old -> new IP.
# Run from operator machine or Back VPS with SSH to OLD_DB, NEW_DB, and local Back .env.
#
# Required env:
#   OLD_DB_HOST=108.174.78.39
#   NEW_DB_HOST=87.228.63.206
#   BACK_HOST=198.13.189.75
#   BACK_VPS_PUBLIC=198.13.189.75
set -euo pipefail

OLD_DB_HOST="${OLD_DB_HOST:?}"
NEW_DB_HOST="${NEW_DB_HOST:?}"
BACK_HOST="${BACK_HOST:?}"
BACK_VPS_PUBLIC="${BACK_VPS_PUBLIC:-198.13.189.75}"
BACK_ENV="${BACK_ENV:-/opt/testVPStrade/infra/docker/.env}"
STAMP="$(date +%Y%m%d_%H%M%S)"
FINAL_DUMP="/var/backups/testVPStrade/vps_platform_final_${STAMP}.sql.gz"

echo "=== Phase 1: final backup on old DB (services still running) ==="
ssh "root@${OLD_DB_HOST}" "mkdir -p /var/backups/testVPStrade && docker exec postgres pg_dump -U vps -d vps_platform --no-owner --no-acl --clean --if-exists | gzip > ${FINAL_DUMP} && gzip -t ${FINAL_DUMP} && ls -lah ${FINAL_DUMP}"

echo "=== Phase 2: stop Back stack (writers paused) ==="
ssh "root@${BACK_HOST}" "cd /opt/testVPStrade/infra/docker && docker compose -f docker-compose.back.yml stop"

echo "=== Phase 3: sync dump old -> new and restore ==="
ssh "root@${OLD_DB_HOST}" "cat ${FINAL_DUMP}" | ssh "root@${NEW_DB_HOST}" "cat > /tmp/vps_platform_final.sql.gz && gzip -t /tmp/vps_platform_final.sql.gz && zcat /tmp/vps_platform_final.sql.gz | docker exec -i postgres psql -U vps -d vps_platform -v ON_ERROR_STOP=1"

echo "=== Phase 4: verify counts on new DB ==="
OLD_M="$(ssh root@${OLD_DB_HOST} "docker exec postgres psql -U vps -d vps_platform -At -c \"SELECT count(*) FROM platform.schema_migrations;\"")"
NEW_M="$(ssh root@${NEW_DB_HOST} "docker exec postgres psql -U vps -d vps_platform -At -c \"SELECT count(*) FROM platform.schema_migrations;\"")"
echo "migrations old=${OLD_M} new=${NEW_M}"
[[ "$OLD_M" == "$NEW_M" ]] || { echo "migration count mismatch" >&2; exit 1; }

echo "=== Phase 5: update Back .env ==="
ssh "root@${BACK_HOST}" "sed -i \"s|^DB_VPS_IP=.*|DB_VPS_IP=${NEW_DB_HOST}|\" ${BACK_ENV} && sed -i \"s|@[0-9.]*:5432|@${NEW_DB_HOST}:5432|g\" ${BACK_ENV} && sed -i \"s|@[0-9.]*:6379|@${NEW_DB_HOST}:6379|g\" ${BACK_ENV} && grep -E '^DB_VPS_IP=|^POSTGRES_DSN=|^REDIS_URL=' ${BACK_ENV} | sed 's/:[^:@]*@/:***@/g'"

echo "=== Phase 6: start Back stack ==="
ssh "root@${BACK_HOST}" "cd /opt/testVPStrade && bash infra/scripts/deploy-back.sh"

echo "=== Phase 7: health ==="
ssh "root@${BACK_HOST}" "docker exec docker-gateway-1 wget -qO- http://127.0.0.1:8080/health || curl -sf http://192.168.0.2:8080/health"

echo "=== Cutover complete: ${OLD_DB_HOST} -> ${NEW_DB_HOST} ==="
