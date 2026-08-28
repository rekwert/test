#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
FI_PASS="$FI_SSH_PASS"

echo "========== STABILIZE: FI + NL + restore portal =========="

echo "=== 1. Fix FI SSH host key on NL panel (root cause of CONNECTION ERROR) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<NL
set -euo pipefail
ssh-keygen -f /root/.ssh/known_hosts -R '95.216.1.155' 2>/dev/null || true
ssh-keyscan -H 95.216.1.155 >> /root/.ssh/known_hosts 2>/dev/null || true
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${FI_PASS}'
sshpass -e ssh -o StrictHostKeyChecking=no root@95.216.1.155 \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@95.216.1.155 'hostname; test -d /opt/virtfusion/app/hypervisor && echo AGENT_OK'
# Update FI agent to match panel version
ssh -o BatchMode=yes root@95.216.1.155 'cd /opt/virtfusion/app/hypervisor && bash update 2>&1 | tail -3; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2' || true
NL

echo "=== 2. Fix FI host key on back server too ==="
ssh-keygen -f /root/.ssh/known_hosts -R '95.216.1.155' 2>/dev/null || true
ssh-keyscan -H 95.216.1.155 >> /root/.ssh/known_hosts 2>/dev/null || true
SSHPASS="$FI_PASS" sshpass -e ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=no root@95.216.1.155 'hostname' && echo FI_SSH_OK || echo FI_SSH_FAIL

echo "=== 3. Sync VF agent versions (NL local + DE + GB) — match control panel ==="
# NL panel local hypervisor agent
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'test -d /opt/virtfusion/app/hypervisor && cd /opt/virtfusion/app/hypervisor && bash update 2>&1 | tail -4 || echo no-local-agent'
# DE-prosto + GB only (skip DE-mid until KVM)
for spec in "185.84.224.84:$DE_SSH_PASS" "212.108.83.47:$GB_SSH_PASS"; do
  IFS=: read -r ip pass <<< "$spec"
  SSHPASS="$pass" sshpass -e ssh -o ConnectTimeout=20 -o StrictHostKeyChecking=no root@$ip \
    'cd /opt/virtfusion/app/hypervisor && bash update 2>&1 | tail -2' || echo "skip $ip"
done

echo "=== 4. VF DB: all hypervisors enabled, FI commissioned ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
for ID in 1 2 3 4; do
  "${MY[@]}" -e "UPDATE hypervisors SET enabled=1, maintenance=0, commissioned=3 WHERE id=$ID;"
done
# DE-mid stays enabled in VF but unreachable until KVM
"${MY[@]}" -e "DELETE FROM ss_alert_log; DELETE FROM ss_alert_lock_log;" 2>/dev/null || true
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -4 || true
"${MY[@]}" -e "SELECT id,name,enabled,commissioned FROM hypervisors WHERE id IN (1,2,3,4);"
NL

echo "=== 5. Portal: restore FI tiers, DE-mid offline until KVM ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes SET
  status = 'online', maintenance_mode = false, vf_enabled = true, vf_commissioned = 3,
  supported_tiers = ARRAY['prosto','midrange']::text[], updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002';

UPDATE vps.nodes SET status = 'offline', updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

UPDATE vps.nodes SET
  status = 'online', maintenance_mode = false, vf_enabled = true, vf_commissioned = 3,
  updated_at = now()
WHERE id IN (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb004'
);

SELECT name, region, status, supported_tiers FROM vps.nodes ORDER BY region;
SQL

echo "=== 6. Restart worker ==="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "=== 7. Verify FI from NL ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'ssh -o BatchMode=yes -o ConnectTimeout=10 root@95.216.1.155 "supervisorctl status 2>/dev/null | head -3; curl -sk -o /dev/null -w %{http_code} https://127.0.0.1:8892/health"'

echo "STABILIZE_DONE"
