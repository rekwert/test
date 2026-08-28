#!/bin/bash
echo "=== VFCLI SERVER COMMANDS ==="
/opt/virtfusion/php/bin/php /opt/virtfusion/app/control/artisan list 2>/dev/null | grep -i server | head -30

echo "=== TRY DELETE SERVER 35 VIA ARTISAN ==="
/opt/virtfusion/php/bin/php /opt/virtfusion/app/control/artisan server:delete 35 2>&1 | head -5

echo "=== HYPERVISOR STORAGE - insert missing row from UI config ==="
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT COUNT(*) FROM hypervisor_storage;" 2>/dev/null
