#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
PHP=/opt/virtfusion/php8/bin/php
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== VF SERVICES ==="
systemctl list-units --type=service | grep -iE 'virt|vf-' | head -20
supervisorctl status 2>/dev/null | head -15

echo "=== AUTH.JSON ==="
cat /opt/virtfusion/app/hypervisor/conf/auth.json 2>/dev/null | head -c 500
echo ""

echo "=== HYPERVISOR AGENT TEST ==="
# control uses encrypted token; try local hypervisor routes
curl -sk "https://127.0.0.1:8892/status" 2>/dev/null | head -c 500
echo ""

echo "=== TRIGGER RESOURCE UPDATE via control ==="
cd /opt/virtfusion/app/control
# dispatch job manually if exists
$PHP artisan tinker --execute="dispatch(new \\App\\Jobs\\Hypervisor\\UpdateResources(1));" 2>&1 | head -10 || true

echo "=== RESTART HYPERVISOR PHP ==="
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3
ls /etc/supervisor/conf.d/ 2>/dev/null
grep -l hypervisor /etc/supervisor/conf.d/* 2>/dev/null

echo "=== RECENT ALLOCATION LOG ==="
myG "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 5;"

echo "=== COMMISSIONED ==="
myG "SELECT commissioned FROM hypervisors WHERE id=1;"
