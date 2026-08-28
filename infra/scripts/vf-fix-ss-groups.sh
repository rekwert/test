#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== SS / HV GROUP LINK TABLES ==="
for t in ss_group_hv_group ss_grp_hv_grp_pkg_grp hypervisor_group_hypervisor ss_package_group ss_package_group_package; do
  echo "--- $t ---"
  myG "SELECT * FROM $t;" 2>/dev/null || echo "(missing)"
done

echo "=== GLOBAL API TOKEN ==="
myG "SELECT id,name,enabled,permissions,access FROM global_api_tokens\G"

echo "=== LICENSE ==="
myG "SELECT \`key\`,value FROM sys_settings WHERE \`key\` LIKE 'license%' OR \`key\` LIKE 'trial%';"

echo "=== HYPERVISORS ==="
myG "SELECT id,name,enabled,commissioned,maintenance,nf_type FROM hypervisors\G"

echo "=== IP BLOCK DETAILS ==="
myG "SELECT * FROM ip_blocks\G"
myG "SELECT COUNT(*) cnt FROM ip_addresses WHERE block_id=1 AND assigned=0;"

echo "=== OS TEMPLATES / MEDIA ==="
myG "SELECT COUNT(*) os_tpl FROM os_templates;"
myG "SELECT id,name,enabled FROM os_templates LIMIT 5;"

echo "=== ENABLE STORAGE1 ON PACKAGE ==="
myG "UPDATE server_package_settings SET value='1' WHERE package_id=1 AND \`key\`='storage1.enabled';"
myG "SELECT \`key\`,value FROM server_package_settings WHERE package_id=1 AND \`key\` LIKE 'storage%enabled';"

echo "=== INSERT SS PACKAGE GROUP IF MISSING ==="
cnt=$(myG "SELECT COUNT(*) FROM ss_package_group;" | tail -1)
if [ "$cnt" = "0" ]; then
  myG "INSERT INTO ss_package_group (name,description,created_at,updated_at) VALUES ('Default','Default',NOW(),NOW());"
  gid=$(myG "SELECT id FROM ss_package_group ORDER BY id DESC LIMIT 1;" | tail -1)
  myG "INSERT INTO ss_package_group_package (package_group_id,package_id,\`order\`) VALUES ($gid,1,1);"
  echo "created ss package group $gid"
fi
myG "SELECT * FROM ss_package_group;"
myG "SELECT * FROM ss_package_group_package;"

echo "=== INSERT SS HV GROUP LINK IF MISSING ==="
cnt=$(myG "SELECT COUNT(*) FROM ss_group_hv_group;" | tail -1)
if [ "$cnt" = "0" ]; then
  myG "INSERT INTO ss_group_hv_group (ss_group_id,hv_group_id,created_at,updated_at) VALUES (1,1,NOW(),NOW());" 2>/dev/null || \
  myG "DESCRIBE ss_group_hv_group;"
fi
myG "SELECT * FROM ss_group_hv_group;" 2>/dev/null

echo "=== DESCRIBE ss tables ==="
myG "DESCRIBE ss_group_hv_group;" 2>/dev/null
myG "DESCRIBE ss_grp_hv_grp_pkg_grp;" 2>/dev/null
