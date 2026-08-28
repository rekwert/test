#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== HYPERVISOR COLUMNS ==="
myG "SHOW COLUMNS FROM hypervisors;" | grep -iE 'sum|alloc|count|server|cpu|memory'

echo "=== SERVERS TABLE ==="
myG "SELECT id,state,hypervisor_id,commissioned FROM servers;"
myG "SELECT state,COUNT(*) c FROM servers GROUP BY state;"

echo "=== ORPHAN RESOURCE ROWS ==="
for t in server_disks server_config server_disks_storage; do
  echo "--- $t ---"
  myG "SELECT COUNT(*) FROM $t;" 2>/dev/null
done

echo "=== HYPERVISOR FULL NUMERIC ==="
myG "SELECT id,name,max_cpu,max_memory,max_servers,max_local_hdd FROM hypervisors WHERE id=1\G"

echo "=== SEARCH sum_ in DB ==="
myG "SELECT TABLE_NAME,COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='$DB_DATABASE' AND COLUMN_NAME LIKE 'sum_%';"

echo "=== SYS COUNTERS ==="
myG "SELECT * FROM sys_counters;"

echo "=== TRY RESET via SQL if columns exist ==="
myG "UPDATE hypervisors SET max_cpu=0, max_servers=0 WHERE id=1;" 2>/dev/null && echo "set limits to unlimited(0)"
myG "SELECT max_cpu,max_servers FROM hypervisors WHERE id=1;"
