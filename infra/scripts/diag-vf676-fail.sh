#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
VF_ID="${1:-676}"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")

echo "=== server ${VF_ID} ==="
"\${MY[@]}" -e "SELECT id,state,commissioned,build_fail,hypervisor_id,package_id,uuid FROM servers WHERE id=${VF_ID}\\G"

echo "=== hypervisor_jobs for ${VF_ID} ==="
"\${MY[@]}" -e "SELECT id,hypervisor_id,server_id,action,status,message,created_at FROM hypervisor_jobs WHERE server_id=${VF_ID} ORDER BY id DESC LIMIT 10\\G" 2>/dev/null || \
"\${MY[@]}" -e "DESCRIBE hypervisor_jobs;" | head -20

echo "=== failed_jobs tail ==="
"\${MY[@]}" -e "SELECT id,queue,LEFT(payload,200),exception,failed_at FROM failed_jobs ORDER BY id DESC LIMIT 3\\G" 2>/dev/null | head -80

echo "=== DE-mid deep ==="
ssh -o BatchMode=yes root@66.151.40.165 bash -s <<'INNER'
for f in /opt/virtfusion/app/hypervisor/storage/logs/app-*.log; do
  [[ -f "$f" ]] || continue
  echo "FILE $f lines=$(wc -l < "$f")"
  grep -iE '676|error|fail|build|libvirt|storage|commission' "$f" 2>/dev/null | tail -20
done
echo "=== virsh ==="
virsh list --all
echo "=== disk dir ==="
ls -la /home/vf-data/disk/ /home/vf-data/server/ 2>/dev/null
echo "=== health local ==="
python3 -c "import json; t=json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token']; print('token_len',len(t))" 2>/dev/null
curl -sk -m 8 https://127.0.0.1:8892/health -H "Authorization: Bearer $(python3 -c "import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])")" -w "\nHTTP=%{http_code}\n" 2>/dev/null || true
supervisorctl status
INNER

echo "=== NL health to DE-mid ==="
curl -sk -m 10 "https://66.151.40.165:8892/health" \
  -H "Authorization: Bearer $(ssh -o BatchMode=yes root@66.151.40.165 python3 -c "import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])")" \
  -w "\nHTTP=%{http_code}\n" 2>/dev/null || echo health_fail
REMOTE

source /opt/testVPStrade/infra/docker/.env
curl -sk "${VIRTFUSION_API_URL}/servers/${VF_ID}" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;s=json.load(sys.stdin)["data"];print("api state",s["state"],"commission",s["commissionStatus"],"buildFailed",s["buildFailed"],"tasks",s.get("tasks"))'
