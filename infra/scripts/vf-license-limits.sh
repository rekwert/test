#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== SYS SETTINGS license/limit ==="
myG "SELECT \`key\`,LEFT(value,120) val FROM sys_settings WHERE \`key\` LIKE '%license%' OR \`key\` LIKE '%limit%' OR \`key\` LIKE '%trial%' OR \`key\` LIKE '%server%';"

echo "=== SYS COUNTERS ALL ==="
myG "SELECT * FROM sys_counters;"

echo "=== IP ADDRESSES ==="
myG "SHOW COLUMNS FROM ip_addresses;" | head -15
myG "SELECT COUNT(*) total FROM ip_addresses;"
myG "SELECT id,address,block_id FROM ip_addresses LIMIT 10;"

echo "=== HYPERVISOR CONFIG ==="
myG "SELECT * FROM hypervisor_config WHERE hypervisor_id=1\G"

echo "=== VFCLI COMMANDS ==="
/opt/virtfusion/vfcli-ctrl list 2>/dev/null | grep -i hypervisor | head -20
