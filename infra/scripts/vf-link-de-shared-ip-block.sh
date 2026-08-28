#!/usr/bin/env bash
# Link shared DE IP block (212.102.227.0/24) to an additional hypervisor (e.g. future DE-prosto node).
# Run on NL panel. Usage: DE_PROSTO_HV_ID=5 bash vf-link-de-shared-ip-block.sh
set -euo pipefail
DE_BLOCK_ID="${DE_BLOCK_ID:-6}"
DE_PROSTO_HV_ID="${DE_PROSTO_HV_ID:?set DE_PROSTO_HV_ID}"
DE_NET_ID="${DE_NET_ID:-}"

set -a
source /opt/virtfusion/app/control/.env
set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

NET_ID="$DE_NET_ID"
if [[ -z "$NET_ID" ]]; then
  NET_ID=$("${MY[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=$DE_PROSTO_HV_ID AND \`primary\`=1 LIMIT 1;")
fi
[[ -n "$NET_ID" ]] || { echo "no primary network for HV $DE_PROSTO_HV_ID"; exit 1; }

"${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES ($DE_BLOCK_ID,$DE_PROSTO_HV_ID);"
"${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES ($DE_BLOCK_ID,$NET_ID,$DE_PROSTO_HV_ID);"

echo "linked block $DE_BLOCK_ID to HV $DE_PROSTO_HV_ID (shared DE pool — VF prevents duplicate IP assign)"
"${MY[@]}" -e "SELECT ibh.hypervisor_id, ib.name FROM ip_block_hypervisor ibh JOIN ip_blocks ib ON ib.id=ibh.block_id WHERE ib.id=$DE_BLOCK_ID;"
