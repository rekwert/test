#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }
echo "=== token_authentication ==="
myG "SELECT * FROM token_authentication\G"
echo "=== global_api_tokens full structure ==="
myG "SELECT id,name,enabled,permissions,access,CHAR_LENGTH(token_1) l1,CHAR_LENGTH(token_2) l2,used_at FROM global_api_tokens\G"
