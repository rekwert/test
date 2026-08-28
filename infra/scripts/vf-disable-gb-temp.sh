#!/usr/bin/env bash
# Temporarily disable GB hypervisor (VF license = 3 slots, GB is 4th).
# Run on back server. Re-enable with vf-enable-gb-prosto.sh when license upgraded.
set -euo pipefail

GB_HV_ID="${GB_HV_ID:-4}"
NL_SSH_PASS="${NL_SSH_PASS:-zx_zvJdI9P}"

echo "=== 1. VF NL: disable GB hypervisor ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 'bash -s' <<REMOTE
set -euo pipefail
source /opt/virtfusion/app/control/.env
mysql -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" <<SQL
UPDATE hypervisors
SET enabled=0, prohibit=1, maintenance=1, updated_at=NOW()
WHERE id=${GB_HV_ID};
SELECT id,name,ip,enabled,prohibit,maintenance,commissioned FROM hypervisors ORDER BY id;
SQL
REMOTE

echo "=== 2. Portal: disable GB-1 node ==="
cd /opt/testVPStrade/infra/docker
set -a; source .env; set +a
export PGPASSWORD="$POSTGRES_PASSWORD"
psql -h 108.174.78.39 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<'SQL'
UPDATE vps.nodes
SET vf_enabled = false,
    maintenance_mode = true,
    status = 'maintenance',
    updated_at = now()
WHERE region = 'gb';
SELECT name, region, status, vf_enabled, maintenance_mode, external_id FROM vps.nodes WHERE region = 'gb';
SQL

echo "=== 3. Remove gb from worker provision regions ==="
CUR=$(grep '^VIRTFUSION_PROVISION_REGIONS=' .env | cut -d= -f2-)
NEW=$(echo "$CUR" | tr ',' '\n' | grep -vi '^gb$' | paste -sd, -)
if [[ "$NEW" == "$CUR" ]]; then
  echo "gb already absent from VIRTFUSION_PROVISION_REGIONS"
else
  sed -i "s|^VIRTFUSION_PROVISION_REGIONS=.*|VIRTFUSION_PROVISION_REGIONS=${NEW}|" .env
  echo "updated: VIRTFUSION_PROVISION_REGIONS=${NEW}"
fi

docker compose -f docker-compose.back.yml --env-file .env up -d vps-worker vps
sleep 4

echo "=== 4. Worker log (license errors should stop) ==="
docker logs docker-vps-worker-1 --since 30s 2>&1 | grep -iE 'license|63b27875|allocate|POST /servers' | tail -8 || echo "(no recent provision lines)"

echo VF_GB_TEMP_DISABLED
