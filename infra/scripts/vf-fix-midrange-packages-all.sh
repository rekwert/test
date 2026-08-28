#!/usr/bin/env bash
# Align VirtFusion server_packages with portal Midrange catalog (dedicated pkgs 17-21, 10).
# Run on VF panel (66.248.206.14) as root.
set -euo pipefail
source /opt/virtfusion/app/control/.env
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

upsert_midrange() {
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
FROM server_packages WHERE id=1 LIMIT 1;"
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
      SELECT media_template_id, $id FROM media_template_server_package WHERE server_package_id=1;"
  fi
  if "${MY[@]}" -e "DESCRIBE os_tpl_collection_server_package;" &>/dev/null; then
    "${MY[@]}" -e "INSERT IGNORE INTO os_tpl_collection_server_package (collection_id, \`order\`, package_id)
      SELECT collection_id, \`order\`, $id FROM os_tpl_collection_server_package WHERE package_id=1;"
  fi
  for GRP in 1 2 3 4 5; do
    "${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
      VALUES ($GRP, $id);" 2>/dev/null || true
  done
}

echo "=== Before (Midrange pkgs) ==="
"${MY[@]}" -e "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id IN (10,17,18,19,20,21) ORDER BY id;"

echo "=== Fix Midrange 1-6 ==="
upsert_midrange 17 MIDRANGE-1 1 2048 40
upsert_midrange 18 MIDRANGE-2 2 4096 40
upsert_midrange 19 MIDRANGE-3 4 6144 60
upsert_midrange 20 MIDRANGE-4 4 8192 100
upsert_midrange 21 MIDRANGE-5 6 12288 120
upsert_midrange 10 MIDRANGE-6 8 24576 150

for id in 10 17 18 19 20 21; do
  link_pkg "$id"
done

echo "=== After ==="
"${MY[@]}" -e "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id IN (10,17,18,19,20,21) ORDER BY id;"
echo VF_MIDRANGE_PACKAGES_FIXED
