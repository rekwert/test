#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== HYPERVISOR STATE ==="
myG "SELECT id,name,commissioned,enabled,maintenance FROM hypervisors\G"

echo "=== HYPERVISOR JOBS ==="
myG "SELECT * FROM hypervisor_jobs ORDER BY id DESC LIMIT 10;" 2>/dev/null
myG "DESCRIBE hypervisor_jobs;" 2>/dev/null

echo "=== HYPERVISOR TASKS ==="
myG "SELECT * FROM hypervisor_tasks ORDER BY id DESC LIMIT 10;"

echo "=== QUEUE HV LOG ==="
supervisorctl tail -5000 vf-queue-hv:00 2>/dev/null | tail -60

echo "=== QUEUE LOG ==="
supervisorctl tail -3000 vf-queue:00 2>/dev/null | tail -40

echo "=== SET commissioned=3 if stuck ==="
# commissioned values: 0=none, 1=?, 2=?, 3=complete
myG "UPDATE hypervisors SET commissioned=3 WHERE id=1 AND commissioned!=3;"
myG "SELECT commissioned FROM hypervisors WHERE id=1;"
