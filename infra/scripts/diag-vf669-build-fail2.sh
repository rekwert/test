#!/bin/bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'REMOTE'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

echo "=== HV5 group ==="
"${MY[@]}" -e "SELECT id,name,hypervisor_group_id FROM hypervisors WHERE id=5\G"

echo "=== package group tables ==="
"${MY[@]}" -e "SHOW TABLES LIKE '%package%';" | head -25

echo "=== package 18 groups ==="
"${MY[@]}" -e "SELECT * FROM server_package_hypervisor_group WHERE server_package_id=18;" 2>/dev/null || \
"${MY[@]}" -e "SELECT * FROM hypervisor_group_packages WHERE package_id=18;" 2>/dev/null || \
"${MY[@]}" -e "DESCRIBE server_packages;" | head -20

echo "=== template 7 ==="
"${MY[@]}" -e "SELECT id,name,enabled,group_id FROM operating_system_templates WHERE id=7\G"

echo "=== try build from localhost ==="
TOKEN=$(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT token FROM global_api_tokens WHERE enabled=1 LIMIT 1;")
curl -sk -w "\nHTTP:%{http_code}\n" -X POST "https://127.0.0.1/api/v1/servers/669/build" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"operatingSystemId":7,"hostname":"vps-gm4kgz.local"}' || true

echo "=== DE-mid files ==="
ssh -o BatchMode=yes root@66.151.40.165 bash -s <<'INNER'
ls -la /opt/virtfusion/app/hypervisor/storage/logs/ 2>/dev/null | tail -5
ls -la /home/vf-data/disk/ 2>/dev/null | head -10
journalctl -u libvirtd --since '30 min ago' --no-pager 2>/dev/null | tail -15
INNER

echo "=== queue jobs recent ==="
"${MY[@]}" -e "SELECT id,queue,status,created_at FROM jobs ORDER BY id DESC LIMIT 5;" 2>/dev/null || \
"${MY[@]}" -e "SHOW TABLES LIKE '%job%';" | head -10
REMOTE
