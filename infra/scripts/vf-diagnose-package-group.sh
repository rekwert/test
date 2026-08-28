#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== PACKAGE GROUP LINK TABLES ==="
myG "SELECT * FROM ss_package_group;"
myG "SELECT * FROM ss_package_group_package;"
myG "SELECT * FROM server_package_hv_asset_grps;"
myG "DESCRIBE ss_package_group;"
myG "DESCRIBE ss_package_group_package;"

echo "=== HYPERVISOR GROUPS ==="
myG "SELECT * FROM hypervisor_groups\G"

echo "=== GREP API ERRORS LANGUAGE ==="
grep -r 'EC 8\|"EC' /opt/virtfusion/app/control/language/en/api.json 2>/dev/null | head -20
grep -r 'EC 8' /opt/virtfusion/app/control/language/ 2>/dev/null | head -10

echo "=== PHP LOG TAIL ==="
tail -30 /opt/virtfusion/php8/var/log/php.log 2>/dev/null
