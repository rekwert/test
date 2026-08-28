#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== ARTISAN LIST ==="
cd /opt/virtfusion/app/control
runuser -u virtfusion -- php artisan list 2>/dev/null | grep -iE 'hypervisor|license|resource|commission' | head -40

echo "=== LICENSE TABLES ==="
myG "SHOW TABLES LIKE '%licen%';"
myG "SELECT * FROM license LIMIT 5;" 2>/dev/null
myG "SELECT * FROM licenses LIMIT 5;" 2>/dev/null
myG "SELECT * FROM vf_license LIMIT 5;" 2>/dev/null

echo "=== SYS MON HYPERVISOR ==="
myG "SELECT * FROM sys_mon_hypervisor\G"

echo "=== HYPERVISOR FULL ROW ==="
myG "SELECT * FROM hypervisors WHERE id=1\G"

echo "=== SETTINGS TABLE ==="
myG "SHOW TABLES LIKE '%setting%';"
myG "SELECT \`key\`,LEFT(value,200) FROM settings WHERE \`key\` LIKE '%licen%' LIMIT 20;" 2>/dev/null
myG "SELECT \`key\`,LEFT(value,200) FROM global_settings WHERE \`key\` LIKE '%licen%' LIMIT 20;" 2>/dev/null

echo "=== TRY hypervisor resource update jobs ==="
runuser -u virtfusion -- php artisan 2>&1 | head -5
for cmd in "hypervisor:resources:sync 1" "hypervisor:sync 1" "compute:hypervisor:sync 1" "vf:hypervisor:sync 1"; do
  echo "TRY: $cmd"
  runuser -u virtfusion -- php artisan $cmd 2>&1 | head -5
done

echo "=== DECODE IPV4 ==="
python3 - <<'PY'
import socket,struct
for n in [1123601960,1123601981]:
    print(n, socket.inet_ntoa(struct.pack('!I', n)))
PY
