#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== OS TPL COLLECTIONS ==="
myG "SELECT * FROM os_tpl_collections;"
myG "SELECT * FROM os_tpl_collection_server_package;"
myG "SELECT * FROM os_tpl_collection_os_tpl LIMIT 20;"
myG "SELECT * FROM os_tpl_download LIMIT 10;"
myG "SELECT * FROM operating_system_isos LIMIT 5;"

echo "=== RE-COMMISSION with yes ==="
printf '1\nyes\n' | $PHP artisan hypervisor:re-commission 2>&1 | tail -30

sleep 10
echo "=== HYPERVISOR AFTER ==="
myG "SELECT id,name,commissioned,enabled,nf_type FROM hypervisors;"
