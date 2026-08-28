#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== ALL USERS ==="
myG "SELECT id,name,email,admin,suspended FROM users ORDER BY id LIMIT 20;"
echo "=== USER COUNT ==="
myG "SELECT COUNT(*) AS cnt FROM users;"
echo "=== SERVERS BY STATE ==="
myG "SELECT state, COUNT(*) c FROM servers GROUP BY state;"
echo "=== SERVERS RECENT ==="
myG "SELECT id,owner_id,hypervisor_id,state,commission_status,name FROM servers ORDER BY id DESC LIMIT 15;"
echo "=== PACKAGE GROUPS ==="
myG "SELECT * FROM ss_package_group;"
myG "SELECT * FROM ss_package_group_package;"
myG "SELECT COUNT(*) AS storage_rows FROM hypervisor_storage;"
echo "=== LATEST FAIL LOG DETAIL ==="
myG "SELECT id,message,allocation,created_at FROM log_resource_allocation WHERE success=0 ORDER BY id DESC LIMIT 1\G"
