#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control

echo "=== OS TEMPLATES ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SHOW TABLES LIKE '%os%';" 2>/dev/null
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT COUNT(*) c FROM os_templates;" 2>/dev/null
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,name,enabled FROM os_templates LIMIT 10;" 2>/dev/null
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT COUNT(*) c FROM os_tpl;" 2>/dev/null

echo "=== SERVER PACKAGES enabled ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,name,enabled,type FROM server_packages;"

echo "=== RE-COMMISSION HYPERVISOR 1 ==="
printf '1\n' | $PHP artisan hypervisor:re-commission 2>&1 | head -40

echo "=== NAT SYNC ==="
$PHP artisan nat:sync-hypervisor 1 2>&1 | head -20

sleep 8
echo "=== DONE ==="
