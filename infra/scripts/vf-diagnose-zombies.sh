#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== SERVER 35 DETAIL ==="
myG "SELECT id,state,commissioned,uuid,package_id,hypervisor_id,destroyable FROM servers WHERE id=35\G"
myG "SELECT COUNT(*) disks FROM server_disks WHERE server_id=35;"
myG "SELECT COUNT(*) cfg FROM server_config WHERE server_id=35;"

echo "=== LICENSE TYPE MEANING (hypervisor) ==="
myG "SELECT id,license_type,max_servers FROM hypervisors WHERE id=1\G"

echo "=== COMMISSIONED BREAKDOWN ==="
myG "SELECT commissioned, COUNT(*) c FROM servers GROUP BY commissioned;"
