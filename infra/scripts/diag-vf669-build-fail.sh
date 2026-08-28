#!/bin/bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'REMOTE'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

echo "=== server 669 ==="
"${MY[@]}" -e "SELECT id,state,commissioned,package_id,hypervisor_id FROM servers WHERE id=669;"

echo "=== package 18 + group links ==="
"${MY[@]}" -e "SELECT id,name,enabled FROM server_packages WHERE id=18\G" | head -20
"${MY[@]}" -e "SELECT * FROM hypervisor_group_server_package WHERE server_package_id=18;"

echo "=== HV5 storage/network ==="
"${MY[@]}" -e "SELECT id,name,path,enabled,\`default\` FROM hypervisor_storage WHERE hypervisor_id=5;"
"${MY[@]}" -e "SELECT id,type,bridge,\`primary\`,enabled FROM hypervisor_networks WHERE hypervisor_id=5;"
"${MY[@]}" -e "SELECT * FROM ip_block_hypervisor WHERE hypervisor_id=5;"

echo "=== OS template 7 on HV5 group ==="
"${MY[@]}" -e "SELECT id,name,enabled FROM operating_systems WHERE id=7\G" 2>/dev/null | head -15 || true
"${MY[@]}" -e "SHOW TABLES LIKE '%operating%';" | head -10

echo "=== server_build_log 669 ==="
"${MY[@]}" -e "SELECT * FROM server_build_log WHERE server_id=669 ORDER BY id DESC LIMIT 5\G" 2>/dev/null || true

echo "=== server_action_log 669 ==="
"${MY[@]}" -e "SELECT * FROM server_action_log WHERE server_id=669 ORDER BY id DESC LIMIT 5\G" 2>/dev/null || true

echo "=== log_resource_allocation tail ==="
"${MY[@]}" -e "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 5;"

echo "=== nginx/php errors tail ==="
tail -20 /opt/virtfusion/nginx/logs/error.log 2>/dev/null || true
tail -30 /opt/virtfusion/php8/var/log/php.log 2>/dev/null | grep -iE '669|error|exception|build' | tail -15 || true

echo "=== DE-mid hypervisor log today ==="
ssh -o BatchMode=yes root@66.151.40.165 "LOG=/opt/virtfusion/app/hypervisor/storage/logs/app-\$(date +%Y-%m-%d).log; wc -l \$LOG 2>/dev/null; tail -30 \$LOG 2>/dev/null" || true
REMOTE
