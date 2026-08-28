#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== ALL OS-RELATED DATA ==="
for t in operating_system_isos os_tpl_download os_tpl_provisioner os_tpl_collection_os_tpl server_os_tpl_collections; do
  echo "--- $t count ---"
  myG "SELECT COUNT(*) FROM $t;" 2>/dev/null || echo missing
done

myG "SELECT * FROM operating_system_isos LIMIT 10;"
myG "SELECT * FROM os_tpl_download LIMIT 10;"
myG "DESCRIBE operating_system_isos;" 2>/dev/null | head -20

echo "=== RE-COMMISSION triple yes ==="
printf '1\nyes\nyes\n' | $PHP artisan hypervisor:re-commission 2>&1 | tail -40

sleep 15
