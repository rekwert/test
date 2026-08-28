#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== ALL TABLES license/sys ==="
myG "SHOW TABLES LIKE '%lic%';"
myG "SHOW TABLES LIKE 'sys_%';"

echo "=== SYS SETTINGS ==="
myG "SELECT * FROM sys_settings LIMIT 20;" 2>/dev/null
myG "SELECT * FROM sys_config LIMIT 20;" 2>/dev/null
myG "SELECT * FROM settings LIMIT 20;" 2>/dev/null

echo "=== API TOKENS ==="
myG "SELECT id,user_id,enabled,LEFT(token,20) pfx,created_at FROM user_api_tokens;"

echo "=== LATEST RESOURCE LOG ==="
myG "SELECT id,success,message,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 3;"

echo "=== GREP license in control storage ==="
grep -ri 'license' /opt/virtfusion/app/control/storage/ 2>/dev/null | head -10

echo "=== CONTROL .env license keys ==="
grep -i license /opt/virtfusion/app/control/.env 2>/dev/null | sed 's/=.*/=***/'

echo "=== RESTART QUEUES ==="
supervisorctl restart vf-queue-hv:00 vf-queue-hv:01 vf-queue:00 2>/dev/null | head -5
