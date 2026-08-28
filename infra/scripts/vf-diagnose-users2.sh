#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== USER FULL ==="
myG "SELECT * FROM users\G"

echo "=== SERVERS ALLOCATED SAMPLE ==="
myG "SELECT id,owner_id,hypervisor_id,state,commission_status FROM servers WHERE state='allocated' ORDER BY id LIMIT 10;"
myG "SELECT MIN(id), MAX(id), COUNT(*) FROM servers WHERE state='allocated';"

echo "=== SUM CPU ON HYPERVISOR FROM SERVERS ==="
myG "SELECT SUM(cpu_cores) total_cpu, COUNT(*) cnt FROM servers WHERE hypervisor_id=1 AND state='allocated';"

echo "=== DESCRIBE servers ==="
myG "DESCRIBE servers;" | head -25

echo "=== LATEST 3 RESOURCE LOGS summary ==="
myG "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 5;"
