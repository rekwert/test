#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== BEFORE ==="
myG "SELECT id,name,max_servers,enabled,commissioned FROM hypervisors;"
myG "SELECT * FROM ss_group_hv_group;"
myG "SELECT * FROM ss_grp_hv_grp_pkg_grp;"

echo "=== FIX max_servers (0 blocks allocation) ==="
myG "UPDATE hypervisors SET max_servers=0 WHERE id=1;"
# VF uses 0 as unlimited in UI often; check hypervisor_settings
myG "SELECT * FROM hypervisor_settings WHERE hypervisor_id=1;"

echo "=== LINK self-service groups ==="
cnt=$(myG "SELECT COUNT(*) FROM ss_group_hv_group;" | tail -1)
if [ "$cnt" = "0" ]; then
  myG "INSERT INTO ss_group_hv_group (group_id,hypervisor_group_id,name,label,\`order\`) VALUES (1,1,'Default','Default',1);"
fi
cnt=$(myG "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp;" | tail -1)
if [ "$cnt" = "0" ]; then
  myG "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id,hypervisor_group_id,group_id,\`order\`) VALUES (1,1,1,1);"
fi

echo "=== AFTER LINKS ==="
myG "SELECT * FROM ss_group_hv_group;"
myG "SELECT * FROM ss_grp_hv_grp_pkg_grp;"

echo "=== HYPERVISOR AGENT HEALTH ==="
curl -sk "https://127.0.0.1:8892/health" 2>/dev/null | head -c 500 || curl -sk "http://127.0.0.1:8892/health" 2>/dev/null | head -c 500
echo ""
curl -sk "http://127.0.0.1:8892/" 2>/dev/null | head -c 200
echo ""

echo "=== RESTART QUEUES ==="
supervisorctl restart vf-queue-hv: vf-queue: 2>/dev/null | tail -5

echo "=== LIBVIRT ==="
systemctl is-active libvirtd
virsh list --all 2>/dev/null | head -5

echo "=== TRIGGER HV RESOURCE SYNC via vfcli ==="
echo "1" | /opt/virtfusion/vfcli-ctrl hypervisor:update-resources 2>&1 | head -20 || true
echo "1" | /opt/virtfusion/vfcli-ctrl hypervisor:refresh 2>&1 | head -20 || true

sleep 5
echo "=== SYS COUNTERS ==="
myG "SELECT * FROM sys_counters WHERE name LIKE 'srv_%';"
