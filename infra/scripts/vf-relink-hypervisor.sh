#!/bin/bash
source /opt/virtfusion/app/control/.env
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control

echo "=== RESTORE HYPERVISOR LIMITS FROM HARDWARE ==="
# get real cpu/mem from system
CPUS=$(nproc)
MEM=$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "UPDATE hypervisors SET max_cpu=$CPUS, max_memory=$MEM, max_servers=0 WHERE id=1;"
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT max_cpu,max_memory,max_servers FROM hypervisors WHERE id=1;"

echo "=== FORCE REMOVE STALE AUTH AND RECOMMISSION ==="
cp /opt/virtfusion/app/hypervisor/conf/auth.json /opt/virtfusion/app/hypervisor/conf/auth.json.bak 2>/dev/null || true
rm -f /opt/virtfusion/app/hypervisor/conf/auth.json
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "UPDATE hypervisors SET commissioned=0 WHERE id=1;"

printf '1\nyes\nyes\n' | $PHP artisan hypervisor:re-commission 2>&1 | tail -10

sleep 15

echo "=== AUTH RECREATED? ==="
ls -la /opt/virtfusion/app/hypervisor/conf/auth.json 2>/dev/null || echo "NO AUTH YET - need UI wizard"
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT commissioned FROM hypervisors WHERE id=1;"

supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2
