#!/usr/bin/env bash
# Re-enable GB after VirtFusion license upgraded to 4 hypervisors.
# Run on back server:
#   export GB_SSH_PASS='...'
#   bash infra/scripts/vf-enable-gb-prosto.sh
set -euo pipefail

GB_HV_ID="${GB_HV_ID:-4}"
GB_NET_ID="${GB_NET_ID:-4}"
GB_GROUP_ID="${GB_GROUP_ID:-4}"
GB_HV_IP="${GB_HV_IP:-212.108.83.47}"
NL_SSH_PASS="${NL_SSH_PASS:-zx_zvJdI9P}"
: "${GB_SSH_PASS:?set GB_SSH_PASS}"

echo "=== 1. VF NL: restore GB hypervisor id=${GB_HV_ID} ==="
SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  /opt/testVPStrade/infra/scripts/vf-restore-gb-hypervisor.sh \
  root@66.248.206.14:/tmp/vf-restore-gb-hypervisor.sh 2>/dev/null || \
SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  /tmp/vf-restore-gb-hypervisor.sh root@66.248.206.14:/tmp/vf-restore-gb-hypervisor.sh

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-restore-gb-hypervisor.sh && GB_SSH_PASS='$GB_SSH_PASS' bash /tmp/vf-restore-gb-hypervisor.sh"

echo "=== 2. Portal: enable GB-1 node ==="
cd /opt/testVPStrade/infra/docker
set -a; source .env; set +a
export PGPASSWORD="$POSTGRES_PASSWORD"
psql -h 108.174.78.39 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<'SQL'
UPDATE vps.nodes
SET vf_enabled = true,
    maintenance_mode = false,
    status = 'online',
    vf_commissioned = 3,
    supported_tiers = ARRAY['prosto','midrange','hustle']::text[],
    updated_at = now()
WHERE region = 'gb';
UPDATE vps.regions SET enabled = true, updated_at = now() WHERE code = 'gb';
SELECT name, region, status, vf_enabled, maintenance_mode, external_id, supported_tiers FROM vps.nodes WHERE region = 'gb';
SQL

echo "=== 3. Add gb to VIRTFUSION_PROVISION_REGIONS ==="
CUR=$(grep '^VIRTFUSION_PROVISION_REGIONS=' .env | cut -d= -f2-)
if echo "$CUR" | grep -qiE '(^|,)gb(,|$)'; then
  echo "gb already in: $CUR"
else
  sed -i "s|^VIRTFUSION_PROVISION_REGIONS=.*|VIRTFUSION_PROVISION_REGIONS=${CUR},gb|" .env
  grep '^VIRTFUSION_PROVISION_REGIONS=' .env
fi

docker compose -f docker-compose.back.yml --env-file .env up -d vps-worker vps
sleep 5

echo "=== 4. Verify no license errors ==="
docker logs docker-vps-worker-1 --since 20s 2>&1 | grep -i license | tail -5 || echo "no license errors in last 20s"

echo VF_GB_ENABLED
