#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }
myN() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "$1" 2>/dev/null; }

echo "=== SERVICES ==="
systemctl is-active vf-nginx vf-php8-fpm supervisor libvirtd 2>/dev/null
supervisorctl status 2>/dev/null | grep -E 'FATAL|RUNNING' | head -15

echo "=== HYPERVISOR ==="
myG "SELECT id,name,enabled,commissioned,maintenance,nf_type,license_type,max_servers FROM hypervisors WHERE id=1\G"
myG "SELECT COUNT(*) AS storage_rows FROM hypervisor_storage WHERE hypervisor_id=1;"
myG "SELECT COUNT(*) AS servers FROM servers;"

echo "=== LICENSE ==="
myG "SELECT \`key\`, value FROM system WHERE \`key\` IN ('licenseLast','licenseKey');"

echo "=== API TOKEN ==="
myG "SELECT id,name,enabled,permissions,access,used_at FROM global_api_tokens\G"

echo "=== PACKAGE + GROUP LINKS ==="
myN "SELECT TABLE_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='${DB_DATABASE}' AND COLUMN_NAME='hypervisor_group_id';"
myG "SELECT * FROM server_packages WHERE id=1\G" | head -25

echo "=== RECENT ALLOCATION LOGS ==="
myG "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 5;"

echo "=== HAProxy ERRORS today ==="
grep -c HAProxy /opt/virtfusion/app/hypervisor/storage/logs/app-$(date +%Y-%m-%d).log 2>/dev/null || echo 0

echo "=== NGINX/PHP errors tail ==="
tail -5 /opt/virtfusion/nginx/logs/error.log 2>/dev/null
