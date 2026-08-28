#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== CHILD ROW COUNTS FOR ZOMBIE SERVERS ==="
for t in server_disks server_disks_storage server_firewall server_config server_build_log server_action_log; do
  c=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT COUNT(*) FROM ${t} WHERE server_id IN (SELECT id FROM servers WHERE commissioned=0 AND state='allocated');" 2>/dev/null)
  echo "$t: $c"
done

echo "=== FK ON servers ==="
myG "SELECT TABLE_NAME,COLUMN_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_NAME='servers' AND TABLE_SCHEMA=DATABASE();"
