#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== LIBVIRT ==="
systemctl is-active libvirtd 2>/dev/null || echo inactive
systemctl start libvirtd 2>/dev/null && echo started || echo start_failed
systemctl is-active libvirtd 2>/dev/null

echo "=== HYPERVISOR FULL ==="
myG "SELECT id,name,enabled,commissioned,maintenance,nf_type,license_type,max_servers,group_id FROM hypervisors\G"

echo "=== HYPERVISOR GROUP MEMBERSHIP ==="
myG "SELECT * FROM hypervisor_group_hypervisor;" 2>/dev/null || myG "SHOW TABLES LIKE '%group%hypervisor%';"

echo "=== USERS ==="
myG "SELECT id,name,email,enabled,self_service FROM users;"

echo "=== PACKAGE SETTINGS ==="
myG "SELECT * FROM server_package_settings WHERE package_id=1\G"

echo "=== PACKAGE HV ASSET GRPS ==="
myG "SELECT * FROM server_package_hv_asset_grps;"

echo "=== IP BLOCKS ==="
myG "SELECT id,name,enabled FROM ip_blocks LIMIT 10;"

echo "=== HYPERVISOR NETWORKS ==="
myG "SELECT id,hypervisor_id,name,enabled,bridge FROM hypervisor_networks\G"

echo "=== HYPERVISOR STORAGE ==="
myG "SELECT * FROM hypervisor_storage\G"

echo "=== NAT DOMAINS ==="
myG "SHOW TABLES LIKE '%nat%';"
myG "SELECT * FROM nat_domains LIMIT 5;" 2>/dev/null

echo "=== RECENT RESOURCE ALLOCATION (failures) ==="
myG "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 10;"

echo "=== TRY VFCLI STATUS ==="
/opt/virtfusion/vfcli-ctrl hypervisor:list 2>/dev/null | head -20 || true
