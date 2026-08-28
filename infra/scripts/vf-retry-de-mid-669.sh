#!/bin/bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
DE_MID_PASS="${DE_MID_SSH_PASS:?}"
INSTANCE_ID="8d8a46f7-c421-4fb8-b540-c0932299df95"
VF_SERVER_ID="669"

echo "=== 1. Ensure libvirtd active on DE-mid ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "ssh -o BatchMode=yes root@66.151.40.165 'systemctl start libvirtd; systemctl is-active libvirtd; virsh uri'"

echo ""
echo "=== 2. Reset VF server 669 for rebuild ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
"\${MY[@]}" -e "UPDATE servers SET state='allocated', commissioned=0, build_fail=0, rebuild=0 WHERE id=${VF_SERVER_ID};"
"\${MY[@]}" -e "SELECT id,state,commissioned,build_fail,hypervisor_id FROM servers WHERE id=${VF_SERVER_ID};"
REMOTE

echo ""
echo "=== 3. Portal instance creating ==="
psql "$POSTGRES_DSN" -c "UPDATE vps.instances SET state='creating', updated_at=now() WHERE id='${INSTANCE_ID}';"

echo ""
echo "=== 4. Trigger VF build ==="
OS_ID=$(curl -sk "${VIRTFUSION_API_URL}/servers/${VF_SERVER_ID}" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["settings"]["osTemplateInstallId"])')
curl -sk -w "\nHTTP:%{http_code}\n" -X POST "${VIRTFUSION_API_URL}/servers/${VF_SERVER_ID}/build" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"operatingSystemId\": ${OS_ID}, \"hostname\": \"vps-gm4kgz.local\"}"

echo ""
sleep 15
curl -sk "${VIRTFUSION_API_URL}/servers/${VF_SERVER_ID}" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;s=json.load(sys.stdin)["data"];print("state",s["state"],"commission",s["commissionStatus"],"tasks",s.get("tasks"))'

echo ""
echo "=== 5. DE-mid virsh ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "ssh -o BatchMode=yes root@66.151.40.165 'virsh list --all | head -8'"

echo "RETRY_669_DONE"
