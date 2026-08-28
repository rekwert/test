#!/bin/bash
# Create standalone VF package: 4 vCPU / 4 GB RAM / 200 GB disk (does not touch existing packages).
# Links Windows + Linux templates like PROSTO-3, attaches to NL+FI hypervisor groups.
set -euo pipefail
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1"; }
myN() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "$1"; }

PKG_ID=13
PKG_NAME="MANUAL-4C4G200"
CPU=4
MEM=4096
DISK=200

echo "=== existing packages ==="
myG "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages ORDER BY id;"

EXIST=$(myN "SELECT COUNT(*) FROM server_packages WHERE id=$PKG_ID;")
if [[ "$EXIST" != "0" ]]; then
  NAME=$(myN "SELECT name FROM server_packages WHERE id=$PKG_ID;")
  if [[ "$NAME" == "$PKG_NAME" ]]; then
    echo "package $PKG_ID already exists as $PKG_NAME — updating specs only"
    myG "UPDATE server_packages SET cpu_cores=$CPU, memory=$MEM, storage=$DISK, enabled=1, updated_at=NOW()
      WHERE id=$PKG_ID;"
  else
    PKG_ID=$(myN "SELECT COALESCE(MAX(id),0)+1 FROM server_packages;")
    echo "id 13 taken, using new id $PKG_ID"
  fi
fi

if [[ "$EXIST" == "0" ]] || [[ "$(myN "SELECT name FROM server_packages WHERE id=$PKG_ID;")" != "$PKG_NAME" ]]; then
  echo "=== insert package $PKG_ID ==="
  myG "
INSERT INTO server_packages (
  id, name, enabled, type, memory, storage, traffic, cpu_cores,
  disk_type, backup_plan_id, storage_type, network_profile, token_value, ss_description,
  network_in_average, network_in_peak, network_in_burst,
  network_out_average, network_out_peak, network_out_burst,
  created_at, updated_at
)
SELECT
  $PKG_ID, '$PKG_NAME', 1, type, $MEM, $DISK, traffic, $CPU,
  disk_type, backup_plan_id, storage_type, network_profile, token_value, 'Manual 4c/4gb/200gb — not for portal catalog',
  network_in_average, network_in_peak, network_in_burst,
  network_out_average, network_out_peak, network_out_burst,
  NOW(), NOW()
FROM server_packages WHERE id=3
ON DUPLICATE KEY UPDATE
  name='$PKG_NAME', cpu_cores=$CPU, memory=$MEM, storage=$DISK, enabled=1, updated_at=NOW();
"
fi

echo "=== link OS templates (copy from package 3 — includes Windows) ==="
if myG "DESCRIBE media_template_server_package;" &>/dev/null; then
  myG "INSERT IGNORE INTO media_template_server_package (media_template_id, server_package_id)
    SELECT media_template_id, $PKG_ID FROM media_template_server_package WHERE server_package_id=3;"
  echo "media_template_server_package links: $(myN "SELECT COUNT(*) FROM media_template_server_package WHERE server_package_id=$PKG_ID;")"
fi
if myG "DESCRIBE os_tpl_collection_server_package;" &>/dev/null; then
  myG "INSERT IGNORE INTO os_tpl_collection_server_package (collection_id, \`order\`, package_id)
    SELECT collection_id, \`order\`, $PKG_ID FROM os_tpl_collection_server_package WHERE package_id=3;"
  echo "os_tpl_collection links: $(myN "SELECT COUNT(*) FROM os_tpl_collection_server_package WHERE package_id=$PKG_ID;")"
fi

echo "=== link hypervisor groups NL + FI ==="
for GRP in 1 2; do
  myG "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
    VALUES ($GRP, $PKG_ID);" 2>/dev/null || true
done

echo "=== verify ==="
myG "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id=$PKG_ID;"
myG "SELECT hypervisor_group_id, server_package_id FROM hypervisor_group_server_package WHERE server_package_id=$PKG_ID;"

echo "PKG_ID=$PKG_ID"
