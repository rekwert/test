#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== user_admin_token ==="
myG "SELECT * FROM user_admin_token\G"

echo "=== system table ==="
myG "SELECT * FROM system\G"

echo "=== sys_counters ==="
myG "SELECT * FROM sys_counters\G"

echo "=== hypervisor after purge ==="
myG "SELECT id,enabled,commissioned,license_type,max_servers FROM hypervisors WHERE id=1\G"
myG "SELECT * FROM hypervisor_storage WHERE hypervisor_id=1\G"

echo "=== NEW ALLOCATION ATTEMPT LOG (trigger via local) ==="
# nothing - just show if new logs after purge
myG "SELECT COUNT(*) FROM log_resource_allocation WHERE id>7346;"
