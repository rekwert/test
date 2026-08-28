#!/bin/bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
DE_MID_PASS="${DE_MID_SSH_PASS:?}"
INSTANCE_ID="${1:-8d8a46f7-c421-4fb8-b540-c0932299df95}"
VF_SERVER_ID="${2:-669}"

echo "=== 1. Fix DE-mid libvirt + KVM ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "ssh -o BatchMode=yes -o ConnectTimeout=15 root@66.151.40.165 bash -s" <<'INNER'
set -euo pipefail
systemctl enable libvirtd 2>/dev/null || true
systemctl start libvirtd 2>/dev/null || service libvirtd start 2>/dev/null || true
# Some installs use virtqemud socket activation
systemctl enable virtqemud 2>/dev/null || true
systemctl start virtqemud 2>/dev/null || true
modprobe kvm 2>/dev/null || true
modprobe kvm_intel 2>/dev/null || modprobe kvm_amd 2>/dev/null || true
mkdir -p /home/vf-data/disk /home/vf-data/server
chmod 755 /home/vf-data /home/vf-data/disk
chown -R virtfusion:virtfusion /home/vf-data 2>/dev/null || true
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3 || true
echo "libvirtd=$(systemctl is-active libvirtd 2>/dev/null || echo unknown)"
echo "virtqemud=$(systemctl is-active virtqemud 2>/dev/null || echo n/a)"
echo "kvm=$(test -e /dev/kvm && echo ok || echo missing)"
virsh uri 2>/dev/null || true
INNER

echo ""
echo "=== 2. NL: server 669 + build log ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
"\${MY[@]}" -e "SELECT id,hypervisor_id,state,built,build_failed,hostname,deleted_at FROM servers WHERE id=${VF_SERVER_ID}\\G"
"\${MY[@]}" -e "SELECT id,server_id,action,status,message,created_at FROM server_build_log WHERE server_id=${VF_SERVER_ID} ORDER BY id DESC LIMIT 10;" 2>/dev/null || true
"\${MY[@]}" -e "SELECT id,server_id,action,status,message,created_at FROM server_action_log WHERE server_id=${VF_SERVER_ID} ORDER BY id DESC LIMIT 10;" 2>/dev/null || true
REMOTE

echo ""
echo "=== 3. VF API: rebuild server ${VF_SERVER_ID} ==="
set -a
source "$ROOT/infra/docker/.env"
set +a
# Get OS template from server settings
OS_ID=$(curl -sk "${VIRTFUSION_API_URL}/servers/${VF_SERVER_ID}" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["data"]["settings"]["osTemplateInstallId"])')
echo "osTemplateInstallId=$OS_ID"
curl -sk -w "\nHTTP:%{http_code}\n" -X POST "${VIRTFUSION_API_URL}/servers/${VF_SERVER_ID}/build" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"operatingSystemId\": ${OS_ID}, \"hostname\": \"vps-gm4kgz.local\"}"

echo ""
echo "=== 4. Reset portal instance to creating ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<SQL
UPDATE vps.instances
SET state = 'creating',
    updated_at = now(),
    provider_meta = provider_meta - 'provision_error' - 'password_sync_phase_start'
WHERE id = '${INSTANCE_ID}';
SQL

echo ""
echo "=== 5. Restart worker ==="
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "FIX_DE_MID_669_DONE"
