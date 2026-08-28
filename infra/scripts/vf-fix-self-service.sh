#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== BEFORE users ==="
myG "SELECT id,name,admin,self_service,enabled FROM users;"

echo "=== ENABLE self_service on admin ==="
myG "UPDATE users SET self_service=1 WHERE id=1;"
myG "SELECT id,name,admin,self_service,enabled FROM users;"

echo "=== Test Connection - update hypervisor updated_at ==="
myG "UPDATE hypervisors SET updated_at=NOW() WHERE id=1;"
