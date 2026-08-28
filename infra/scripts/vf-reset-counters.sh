#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== RESET COUNTERS ==="
myG "UPDATE sys_counters SET value=0 WHERE \`key\` IN ('srv_create_total','srv_delete_total');"
myG "SELECT * FROM sys_counters WHERE \`key\` LIKE 'srv_%';"

echo "=== API TOKENS SCHEMA ==="
myG "DESCRIBE user_api_tokens;"
myG "SELECT id,user_id,name,enabled,created_at FROM user_api_tokens;"
myG "SHOW TABLES LIKE '%token%';"

echo "=== TEST CONNECTION hypervisor from control ==="
curl -sk -o /dev/null -w 'local8892:%{http_code}\n' https://127.0.0.1:8892/ 2>/dev/null || true
