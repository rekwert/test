#!/usr/bin/env bash
# Align VirtFusion server_packages with portal CUSTOM catalog (all regions).
# CUSTOM-1: 2 vCPU / 4 GB RAM / 80 GB disk — Claude Code line (Linux templates from PROSTO-3).
# Run on VF panel (66.248.206.14) as root.
set -euo pipefail
source /opt/virtfusion/app/control/.env
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

PKG_ID=16
PKG_NAME="CUSTOM-1"
CPU=2
MEM=4096
DISK=80

upsert_custom() {
  local id=$1 name=$2 cpu=$3 mem=$4 disk=$5
  local exists
  exists=$("${MYN[@]}" -e "SELECT COUNT(*) FROM server_packages WHERE id=$id;")
  if [[ "$exists" == "0" ]]; then
    "${MY[@]}" -e "
INSERT INTO server_packages (
  id, name, enabled, type, memory, storage, traffic, cpu_cores,
  disk_type, backup_plan_id, storage_type, network_profile, token_value, ss_description,
  network_in_average, network_in_peak, network_in_burst,
  network_out_average, network_out_peak, network_out_burst,
  created_at, updated_at
)
SELECT
  $id, '$name', 1, type, $mem, $disk, traffic, $cpu,
  disk_type, backup_plan_id, storage_type, network_profile, token_value, '$name',
  network_in_average, network_in_peak, network_in_burst,
  network_out_average, network_out_peak, network_out_burst,
  NOW(), NOW()
FROM server_packages WHERE id=3 LIMIT 1;"
    echo "created package $id $name"
  else
    "${MY[@]}" -e "UPDATE server_packages SET name='$name', cpu_cores=$cpu, memory=$mem, storage=$disk, enabled=1, updated_at=NOW() WHERE id=$id;"
    echo "updated package $id $name -> cpu=$cpu mem=$mem disk=$disk"
  fi
}

link_pkg() {
  local id=$1
  if "${MY[@]}" -e "DESCRIBE media_template_server_package;" &>/dev/null; then
    "${MY[@]}" -e "INSERT IGNORE INTO media_template_server_package (media_template_id, server_package_id)
      SELECT media_template_id, $id FROM media_template_server_package WHERE server_package_id=3;"
  fi
  if "${MY[@]}" -e "DESCRIBE os_tpl_collection_server_package;" &>/dev/null; then
    "${MY[@]}" -e "INSERT IGNORE INTO os_tpl_collection_server_package (collection_id, \`order\`, package_id)
      SELECT collection_id, \`order\`, $id FROM os_tpl_collection_server_package WHERE package_id=3;"
  fi
  if "${MY[@]}" -e "DESCRIBE ss_package_group_package;" &>/dev/null; then
    "${MY[@]}" -e "INSERT IGNORE INTO ss_package_group_package (package_group_id, package_id, \`order\`)
      VALUES (1, $id, $id);" 2>/dev/null || \
    "${MY[@]}" -e "INSERT IGNORE INTO ss_package_group_package (package_group_id, package_id)
      SELECT 1, $id FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM ss_package_group_package WHERE package_id=$id);" 2>/dev/null || true
  fi
  for GRP in 1 2 3 4; do
    "${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
      VALUES ($GRP, $id);" 2>/dev/null || true
  done
  if "${MY[@]}" -e "DESCRIBE server_package_settings;" &>/dev/null; then
    "${MY[@]}" -e "INSERT IGNORE INTO server_package_settings (package_id, \`key\`, value)
      SELECT $id, \`key\`, value FROM server_package_settings WHERE package_id=3;" 2>/dev/null || true
    "${MY[@]}" -e "UPDATE server_package_settings SET value='1' WHERE package_id=$id AND \`key\`='storage1.enabled';" 2>/dev/null || true
  fi
}

echo "=== Before ==="
"${MY[@]}" -e "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id=$PKG_ID OR name LIKE 'CUSTOM%';"

echo "=== Fix CUSTOM-1 (pkg $PKG_ID) ==="
upsert_custom "$PKG_ID" "$PKG_NAME" "$CPU" "$MEM" "$DISK"
link_pkg "$PKG_ID"

echo "=== After ==="
"${MY[@]}" -e "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id=$PKG_ID;"
if "${MY[@]}" -e "DESCRIBE hypervisor_group_server_package;" &>/dev/null; then
  "${MY[@]}" -e "SELECT hypervisor_group_id, server_package_id FROM hypervisor_group_server_package WHERE server_package_id=$PKG_ID ORDER BY hypervisor_group_id;"
fi
echo "VF_CUSTOM_PACKAGES_FIXED PKG_ID=$PKG_ID"
