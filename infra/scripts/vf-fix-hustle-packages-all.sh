#!/usr/bin/env bash
# Align VirtFusion server_packages with portal HUSTLE catalog (all regions).
# Run on VF panel (66.248.206.14) as root.
set -euo pipefail
source /opt/virtfusion/app/control/.env
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

upsert_hustle() {
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
  for GRP in 1 2 3 4; do
    "${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
      VALUES ($GRP, $id);" 2>/dev/null || true
  done
}

echo "=== Before ==="
"${MY[@]}" -e "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id BETWEEN 5 AND 8 ORDER BY id;"

echo "=== Fix HUSTLE 1-3 ==="
upsert_hustle 5 HUSTLE-1 1 2048 100
upsert_hustle 6 HUSTLE-2 2 4096 120
upsert_hustle 7 HUSTLE-3 4 6144 150

echo "=== Fix HUSTLE 4-6 (pkg 8/13/14) ==="
upsert_hustle 8 HUSTLE-4 4 8192 180
upsert_hustle 13 HUSTLE-5 6 12288 200
upsert_hustle 14 HUSTLE-6 8 24576 250

for id in 5 6 7 8 13 14; do
  link_pkg "$id"
done

echo "=== After ==="
"${MY[@]}" -e "SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages WHERE id IN (5,6,7,8,13,14,15) ORDER BY id;"
echo VF_HUSTLE_PACKAGES_FIXED
