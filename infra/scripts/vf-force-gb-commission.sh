#!/bin/bash
set -euo pipefail
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1"; }

echo "=== SS / package link tables ==="
for t in ss_group_hv_group ss_grp_hv_grp_pkg_grp hypervisor_group_server_package ss_package_group ss_package_group_package hypervisor_group_hypervisor; do
  echo "--- $t ---"
  myG "SELECT * FROM $t;" 2>/dev/null || echo "(missing)"
done

echo "=== hypervisor groups ==="
myG "SELECT id,name,enabled FROM hypervisor_groups ORDER BY id;"

echo "=== server packages ==="
myG "SELECT id,name,enabled FROM server_packages ORDER BY id;"

echo "=== GB hypervisor before ==="
myG "SELECT id,name,commissioned,enabled FROM hypervisors WHERE id=4\G"

echo "=== Force commissioned=3 (auth.json present on GB) ==="
myG "UPDATE hypervisors SET commissioned=3, enabled=1, maintenance=0, prohibit=0 WHERE id=4;"

echo "=== Link GB group 4 to SS if needed ==="
if myG "DESCRIBE ss_group_hv_group;" &>/dev/null; then
  EXISTS=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hv_group_id=4;" 2>/dev/null || echo 0)
  if [[ "$EXISTS" == "0" ]]; then
    SS_GRP=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT ss_group_id FROM ss_group_hv_group WHERE hv_group_id=3 LIMIT 1;" 2>/dev/null || echo 1)
    myG "INSERT INTO ss_group_hv_group (ss_group_id,hv_group_id,created_at,updated_at) VALUES ($SS_GRP,4,NOW(),NOW());" 2>/dev/null || true
  fi
fi

if myG "DESCRIBE ss_grp_hv_grp_pkg_grp;" &>/dev/null; then
  for PKG in $(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT id FROM server_packages WHERE enabled=1;"); do
    PG=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT package_group_id FROM ss_grp_hv_grp_pkg_grp WHERE hv_group_id=3 LIMIT 1;" 2>/dev/null || echo 1)
    myG "INSERT IGNORE INTO ss_grp_hv_grp_pkg_grp (hv_group_id,package_group_id,created_at,updated_at) VALUES (4,$PG,NOW(),NOW());" 2>/dev/null || true
    break
  done
fi

if myG "DESCRIBE hypervisor_group_server_package;" &>/dev/null; then
  for PKG in $(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT id FROM server_packages WHERE enabled=1;"); do
    myG "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (4,$PKG);"
  done
fi

echo "=== GB hypervisor after ==="
myG "SELECT id,name,commissioned,enabled FROM hypervisors ORDER BY id;"

supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3 || true
echo GB_FORCE_COMMISSION_DONE
