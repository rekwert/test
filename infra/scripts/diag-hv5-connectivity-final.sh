#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

echo "=== failed_jobs latest build ==="
"${MY[@]}" -e "SELECT id,LEFT(exception,400),failed_at FROM failed_jobs ORDER BY id DESC LIMIT 2\G"

echo "=== sys_mon HV5 ==="
"${MY[@]}" -e "SELECT hypervisor_id,type,LEFT(raw_data,200),updated_at FROM sys_mon_hypervisor WHERE hypervisor_id=5 ORDER BY id DESC LIMIT 2\G"

echo "=== hypervisor_config count ==="
"${MY[@]}" -e "SELECT hypervisor_id,COUNT(*) FROM hypervisor_config GROUP BY hypervisor_id;"

echo "=== HV3 vs HV5 row ==="
"${MY[@]}" -e "SELECT id,name,ip,port,commissioned,enabled,nf_type,license_type FROM hypervisors WHERE id IN (3,5);"

echo "=== versions ==="
ssh -o BatchMode=yes root@185.84.224.84 'cat /opt/virtfusion/app/hypervisor/update 2>/dev/null | head -3; cat /opt/virtfusion/app/hypervisor/version 2>/dev/null'
ssh -o BatchMode=yes root@66.151.40.165 'cat /opt/virtfusion/app/hypervisor/update 2>/dev/null | head -3; cat /opt/virtfusion/app/hypervisor/version 2>/dev/null'
cat /opt/virtfusion/app/control/version 2>/dev/null || ls /opt/virtfusion/app/control/ | head -5

echo "=== test build server 679 via queue ==="
"${MY[@]}" -e "SELECT id,state,commissioned FROM servers WHERE id=679;"
NL

source /opt/testVPStrade/infra/docker/.env
curl -sk "${VIRTFUSION_API_URL}/servers/679" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | python3 -c 'import sys,json;s=json.load(sys.stdin)["data"];print(s["state"],s["commissionStatus"],s.get("built"))'

docker logs docker-vps-worker-1 --since 5m 2>&1 | grep -iE '8d8a46f7|679|gm4kgz|retry build' | tail -15
