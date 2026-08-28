#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
DBH="${DB_HOST:-127.0.0.1}"
DBU="${DB_USERNAME:-root}"
DBP="${DB_PASSWORD:-}"
DBN="${DB_DATABASE:-virtfusion}"
myG() { mysql -h"$DBH" -u"$DBU" -p"$DBP" "$DBN" -e "$1" 2>/dev/null; }

echo "=== HYPERVISORS DESCRIBE ==="
myG "DESCRIBE hypervisors;"

echo "=== HYPERVISOR ROW ==="
myG "SELECT * FROM hypervisors WHERE id=1\G"

echo "=== HYPERVISOR STORAGE ALL ==="
myG "SELECT * FROM hypervisor_storage\G"

echo "=== HYPERVISOR STORAGE DESCRIBE ==="
myG "DESCRIBE hypervisor_storage;"

echo "=== USERS id=1 ==="
myG "SELECT id,name,email,admin,suspended,created_at FROM users WHERE id=1\G"
myG "SELECT id,name,email,admin,suspended FROM users LIMIT 5;"

echo "=== FAILED ALLOCATION LOGS ==="
myG "SELECT id,type,message,success,accepted,rejected,created_at FROM log_resource_allocation WHERE success=0 ORDER BY id DESC LIMIT 10;"
myG "SELECT id,type,message,allocation,created_at FROM log_resource_allocation WHERE message LIKE '%EC%' OR message LIKE '%valid%' ORDER BY id DESC LIMIT 5\G"

echo "=== SERVERS TABLE ==="
myG "SELECT COUNT(*) AS cnt FROM servers;"
myG "SELECT id,owner_id,hypervisor_id,state,commission_status FROM servers ORDER BY id DESC LIMIT 10;"

echo "=== GREP EC 8 CONTROL LOGS ==="
find /opt/virtfusion/app/control/storage/logs -type f 2>/dev/null | head
grep -r 'EC 8\|\[EC 8\]' /opt/virtfusion/app/control/storage/logs/ /opt/virtfusion/php8/var/log/ 2>/dev/null | tail -20

echo "=== SEARCH EC IN LANGUAGE ==="
grep -r 'EC 8' /opt/virtfusion/app/control/language/ 2>/dev/null | head -5

echo "=== PACKAGE GROUP LINK ==="
myG "SELECT * FROM ss_package_group_package\G"
myG "SELECT * FROM ss_package_group\G"
