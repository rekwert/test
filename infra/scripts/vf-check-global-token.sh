#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }
echo "=== global_api_tokens ==="
myG "SELECT id,name,enabled,permissions,access,LEFT(token_1,8) t1,used_at FROM global_api_tokens\G"
