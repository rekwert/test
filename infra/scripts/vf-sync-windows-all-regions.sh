#!/usr/bin/env bash
# Sync Windows 10/11 templates to all VirtFusion hypervisor groups (NL/FI/DE/GB).
# Run on VF panel host (66.248.206.14) as root:
#   bash /root/vf-sync-windows-all-regions.sh
#
# Copy from your PC:
#   scp infra/scripts/vf-sync-windows-all-regions.sh root@66.248.206.14:/root/
set -euo pipefail

source /opt/virtfusion/app/control/.env
PHP=/opt/virtfusion/php8/bin/php
VFCTL=/opt/virtfusion/app/control
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

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

echo "=== VF sync Windows — all regions (groups 1=NL 2=FI 3=DE 4=GB) ==="

echo "--- link Windows + Linux templates to PROSTO packages on all groups ---"
for id in 1 2 3 4 9; do
  link_pkg "$id"
done

echo "--- hypervisors before ---"
"${MY[@]}" -e "SELECT id,name,hypervisor_group_id,enabled,commissioned FROM hypervisors ORDER BY id;"

echo "--- os_tpl_download per hypervisor (before) ---"
if "${MY[@]}" -e "DESCRIBE os_tpl_download;" &>/dev/null; then
  "${MY[@]}" -e "SELECT hypervisor_id, os_tpl_id, status FROM os_tpl_download ORDER BY hypervisor_id, os_tpl_id LIMIT 40;" 2>/dev/null || true
fi

echo "--- re-commission all hypervisors (downloads OS templates to nodes) ---"
cd "$VFCTL"
for HV in 1 2 3 4; do
  HV_NAME=$("${MYN[@]}" -e "SELECT name FROM hypervisors WHERE id=$HV;" 2>/dev/null || echo "?")
  echo ">>> hypervisor:re-commission $HV ($HV_NAME)"
  printf '%s\nyes\nyes\n' "$HV" | "$PHP" artisan hypervisor:re-commission 2>&1 | tail -25 || true
  sleep 8
done

echo "--- NAT sync ---"
for HV in 1 2 3 4; do
  "$PHP" artisan nat:sync-hypervisor "$HV" 2>&1 | tail -3 || true
done

echo "--- os_tpl_download after re-commission ---"
if "${MY[@]}" -e "DESCRIBE os_tpl_download;" &>/dev/null; then
  "${MY[@]}" -e "
SELECT d.hypervisor_id, h.name, COUNT(*) cnt
FROM os_tpl_download d
JOIN hypervisors h ON h.id = d.hypervisor_id
GROUP BY d.hypervisor_id, h.name
ORDER BY d.hypervisor_id;" 2>/dev/null || true
fi

echo "=== Done ==="
echo VF_WINDOWS_ALL_REGIONS_SYNC_DONE
