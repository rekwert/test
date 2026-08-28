#!/bin/bash
set -euo pipefail
source /opt/virtfusion/app/control/.env
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control
HV_ID=4
GB_SSH_PASS="${GB_SSH_PASS:?set GB_SSH_PASS}"

myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1"; }

echo "=== Link all enabled packages to GB group 4 ==="
for PKG in $(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT id FROM server_packages WHERE enabled=1 ORDER BY id;"); do
  myG "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (4,$PKG);"
done
myG "SELECT hgsp.server_package_id, sp.name FROM hypervisor_group_server_package hgsp JOIN server_packages sp ON sp.id=hgsp.server_package_id WHERE hgsp.hypervisor_group_id=4 ORDER BY sp.id;"

echo "=== Re-commission GB ==="
myG "UPDATE hypervisors SET commissioned=0, enabled=1, maintenance=0, prohibit=0 WHERE id=$HV_ID;"
printf "${HV_ID}\nyes\nyes\n${GB_SSH_PASS}\n" | $PHP artisan hypervisor:re-commission 2>&1 | tail -40

for i in $(seq 1 12); do
  sleep 10
  COMM=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT commissioned FROM hypervisors WHERE id=$HV_ID;")
  echo "poll $i commissioned=$COMM"
  [[ "$COMM" == "3" ]] && break
done

if [[ "$COMM" != "3" ]]; then
  echo "=== Still not commissioned=3, check allocation then force if accepted ==="
  ACC=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT accepted FROM log_resource_allocation ORDER BY id DESC LIMIT 1;" 2>/dev/null || echo 0)
  echo "last allocation accepted=$ACC"
  if [[ "$ACC" == "1" ]]; then
    myG "UPDATE hypervisors SET commissioned=3 WHERE id=$HV_ID;"
    echo "forced commissioned=3"
  fi
fi

myG "SELECT id,name,commissioned,enabled FROM hypervisors WHERE id=$HV_ID\G"
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3 || true
echo GB_FIX_DONE
