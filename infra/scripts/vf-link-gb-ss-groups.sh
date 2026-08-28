#!/bin/bash
set -euo pipefail
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1"; }

echo "=== ss_group_hv_group before ==="
myG "SELECT * FROM ss_group_hv_group ORDER BY group_id;"

# Mirror DE/NL pattern: add GB hv group 4 to SS group 1 (same as default catalog)
EXISTS=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=4;")
if [[ "$EXISTS" == "0" ]]; then
  NEXT=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  myG "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES ($NEXT, 4, 'GB', 'GB', 4);"
fi

EXISTS2=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=4;")
if [[ "$EXISTS2" == "0" ]]; then
  myG "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 4, (SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=4 LIMIT 1), 4);"
fi

echo "=== ss links after ==="
myG "SELECT * FROM ss_group_hv_group ORDER BY group_id;"
myG "SELECT * FROM ss_grp_hv_grp_pkg_grp ORDER BY hypervisor_group_id;"

echo "=== test allocation to GB group 4 ==="
cd /opt/virtfusion/app/control
/opt/virtfusion/php8/bin/php artisan tinker --execute="
\$r = app('App\\\\Services\\\\ResourceAllocation\\\\ResourceAllocationService')->allocate([
  'hypervisor_group_id' => 4,
  'memory' => 1024,
  'cpu' => 1,
  'storage' => 20,
  'storage_profile' => 0,
  'network_profile' => 0,
  'ipv4' => null,
  'asset_groups' => [],
  'sort_method' => 'LONGEST_SERVER_BUILD_DATE',
]);
echo json_encode(\$r, JSON_PRETTY_PRINT);
" 2>&1 | tail -30 || echo "(tinker test skipped)"

echo SS_GB_LINK_DONE
