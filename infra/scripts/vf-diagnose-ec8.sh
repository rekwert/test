#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== EC 8 IN LANGUAGE ==="
grep -r 'EC 8' /opt/virtfusion/app/control/language/ 2>/dev/null | head -10

echo "=== ORPHAN ALLOCATED SERVERS ==="
myG "SELECT id,owner_id,hypervisor_id,state,name,created_at FROM servers WHERE state='allocated' ORDER BY id;"

echo "=== SERVER RESOURCES TABLE ==="
my "SHOW TABLES LIKE '%server%resource%';" 2>/dev/null || true
myG "SHOW TABLES LIKE 'server_%';" | head -20

echo "=== TRY LOCAL API CREATE (from panel env key if exists) ==="
# use first admin api token hash prefix only
myG "SELECT id,user_id,LEFT(token,12) tok_prefix,enabled FROM user_api_tokens LIMIT 5;"
