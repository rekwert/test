#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== ARTISAN HYPERVISOR COMMANDS ==="
cd /opt/virtfusion/app/control
sudo -u virtfusion php artisan list 2>/dev/null | grep -i hypervisor | head -30

echo "=== IPV4 POOL ==="
myG "SELECT COUNT(*) total FROM ipv4;"
myG "SELECT id,address,block_id,server_id FROM ipv4 LIMIT 15;"
myG "SELECT COUNT(*) free FROM ipv4 WHERE server_id IS NULL OR server_id=0;"

echo "=== IP BLOCK HYPERVISOR LINK ==="
myG "SELECT * FROM ip_block_hypervisor;"
myG "SELECT * FROM ip_block_hypervisor_network;"

echo "=== HYPERVISOR TASKS/JOBS DESCRIBE ==="
myG "DESCRIBE hypervisor_tasks;" 2>/dev/null
myG "SELECT * FROM hypervisor_tasks ORDER BY id DESC LIMIT 5;" 2>/dev/null

echo "=== QUEUE LOGS accept/license ==="
supervisorctl tail -2000 vf-queue-hv:00 2>/dev/null | grep -iE 'accept|license|error|hypervisor|resource|commission' | tail -30

echo "=== TRY ARTISAN SYNC ==="
sudo -u virtfusion php artisan hypervisor:sync-resources 1 2>&1 | head -20 || true
sudo -u virtfusion php artisan hypervisor:update 1 2>&1 | head -20 || true
sudo -u virtfusion php artisan hypervisor:refresh 1 2>&1 | head -20 || true
