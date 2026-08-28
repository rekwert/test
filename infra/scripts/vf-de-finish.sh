#!/usr/bin/env bash
# Final DE setup: stable workaround state + status report.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"

echo "=== 1. VF NL: HV3 ready, cleanup HV5 zombies ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1, maintenance=0, prohibit=0 WHERE id=3;"
"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL;"
for PKG in 10 17 18 19 20 21; do
  "${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (3,$PKG);" 2>/dev/null || true
done
supervisorctl restart vf-queue: 2>/dev/null | tail -2 || true
"${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) tok FROM hypervisors WHERE id IN (3,5);"
NL

echo "=== 2. Portal: DE nodes online (shared VF group 3 until HV5 UI commission) ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
DROP INDEX IF EXISTS vps.nodes_external_id_uidx;
UPDATE vps.nodes SET
  external_id = '3', status = 'online', vf_commissioned = 3, vf_enabled = true,
  maintenance_mode = false, updated_at = now()
WHERE id IN ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005');
SELECT id, name, external_id, supported_tiers, status FROM vps.nodes WHERE region='de' ORDER BY name;
SQL

echo "=== 3. Restart worker ==="
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "=== 4. Final status ==="
bash /tmp/de-status-final.sh 2>/dev/null || true
echo "DE_FINISH_COMPLETE"
