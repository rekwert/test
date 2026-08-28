#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
DBH="${DB_HOST:-127.0.0.1}"
DBU="${DB_USERNAME:-root}"
DBP="${DB_PASSWORD:-}"
DBN="${DB_DATABASE:-virtfusion}"
my() { mysql -h"$DBH" -u"$DBU" -p"$DBP" "$DBN" -N -e "$1" 2>/dev/null; }
myG() { mysql -h"$DBH" -u"$DBU" -p"$DBP" "$DBN" -e "$1" 2>/dev/null; }

echo "=== HYPERVISORS ==="
myG "SELECT id,name,enabled,commissioned,maintenance,accept,prohibit,nf_type,group_id FROM hypervisors WHERE id=1\G"

echo "=== HYPERVISOR NETWORKS ==="
myG "SELECT * FROM hypervisor_networks WHERE hypervisor_id=1\G"

echo "=== HYPERVISOR STORAGE ==="
myG "SELECT * FROM hypervisor_storage WHERE hypervisor_id=1\G"

echo "=== SERVER PACKAGES ==="
myG "SELECT * FROM server_packages WHERE id=1\G"
myG "SELECT * FROM server_package_settings WHERE package_id=1\G"

echo "=== PACKAGE HV GROUPS ==="
myG "SELECT * FROM server_package_hv_asset_grps WHERE package_id=1\G" 2>/dev/null
my "SHOW TABLES LIKE '%package%group%';"
my "SHOW TABLES LIKE '%package%hypervisor%';"

echo "=== IP BLOCK HYPERVISOR ==="
myG "SELECT * FROM ip_block_hypervisor\G"
myG "SELECT * FROM ip_block_hypervisor_network\G"

echo "=== RESOURCE ALLOCATION LOG ==="
myG "SELECT * FROM log_resource_allocation ORDER BY id DESC LIMIT 10\G"

echo "=== USERS (owner) ==="
my "SHOW TABLES LIKE '%user%';"
myG "SELECT id,name,email,admin,suspended FROM users LIMIT 10;" 2>/dev/null
myG "SELECT id,name,email,admin,suspended FROM user_accounts LIMIT 10;" 2>/dev/null

echo "=== LICENSE TABLES ==="
my "SHOW TABLES LIKE '%licen%';"
myG "SELECT * FROM sys_license LIMIT 3\G" 2>/dev/null
myG "SELECT * FROM licenses LIMIT 3\G" 2>/dev/null
myG "SELECT * FROM license LIMIT 3\G" 2>/dev/null

echo "=== HYPERVISOR SETTINGS ==="
myG "SELECT * FROM hypervisor_settings WHERE hypervisor_id=1\G"

echo "=== RECENT HAProxy ERRORS count ==="
grep -c 'HAProxy.php' /opt/virtfusion/app/hypervisor/storage/logs/app-2026-07-12.log 2>/dev/null
