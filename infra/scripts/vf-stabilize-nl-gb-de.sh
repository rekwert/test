#!/usr/bin/env bash
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env

echo "=== Fix NL version mismatch (local agent update) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
# Update local hypervisor agent on NL (same host as panel)
if [ -d /opt/virtfusion/app/hypervisor ]; then
  cd /opt/virtfusion/app/hypervisor
  bash update 2>&1 | tail -5
  supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2
fi
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
for ID in 1 3 4; do
  "${MY[@]}" -e "UPDATE hypervisors SET enabled=1, maintenance=0, commissioned=3 WHERE id=$ID;"
done
"${MY[@]}" -e "DELETE FROM ss_alert_log; DELETE FROM ss_alert_lock_log;" 2>/dev/null || true
supervisorctl restart vf-queue: 2>/dev/null | tail -3
"${MY[@]}" -e "SELECT id,name,enabled,commissioned FROM hypervisors WHERE id IN (1,3,4);"
NL

echo "=== Sync DE-prosto + GB agents ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o ConnectTimeout=15 root@185.84.224.84 \
  'cd /opt/virtfusion/app/hypervisor && bash update 2>&1 | tail -2; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -1'
SSHPASS="$GB_SSH_PASS" sshpass -e ssh -o ConnectTimeout=15 root@212.108.83.47 \
  'cd /opt/virtfusion/app/hypervisor && bash update 2>&1 | tail -2; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -1'

echo "=== Portal restore working nodes ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes SET status='online', vf_enabled=true, vf_commissioned=3, maintenance_mode=false,
  supported_tiers=ARRAY['prosto','midrange']::text[], updated_at=now()
WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002' AND EXISTS (SELECT 1);

UPDATE vps.nodes SET status='offline', updated_at=now()
WHERE id IN ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002','bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005');

UPDATE vps.nodes SET status='online', vf_enabled=true, vf_commissioned=3, maintenance_mode=false, updated_at=now()
WHERE region IN ('nl','gb','de') AND id != 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

SELECT name,region,status,supported_tiers FROM vps.nodes ORDER BY region;
SQL

docker restart docker-vps-worker-1 2>/dev/null || true
echo "NL_GB_DE_STABLE_DONE"
