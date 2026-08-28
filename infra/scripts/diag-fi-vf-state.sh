#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,maintenance,prohibit,max_servers,max_cpu,max_memory,max_local_hdd,hypervisor_group_id,LENGTH(token) token_len FROM hypervisors WHERE id=2;"
"${MY[@]}" -e "SELECT id,hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled FROM hypervisor_networks WHERE hypervisor_id=2;"
"${MY[@]}" -e "SELECT * FROM hypervisor_storage WHERE hypervisor_id=2;"
"${MY[@]}" -e "SELECT ib.* FROM ip_block_hypervisor ibh JOIN ip_blocks ib ON ib.id=ibh.block_id WHERE ibh.hypervisor_id=2;"
"${MY[@]}" -e "SELECT state,COUNT(*) count FROM servers WHERE hypervisor_id=2 AND deleted_at IS NULL GROUP BY state;"
"${MY[@]}" -e "SELECT id,name,state,created_at FROM servers WHERE hypervisor_id=2 AND deleted_at IS NULL ORDER BY id;"
NL

cd /opt/testVPStrade/infra/docker
set -a; source .env; set +a
psql "$POSTGRES_DSN" -c "SELECT i.hostname,i.state,i.ip_address,o.order_number FROM vps.instances i LEFT JOIN vps.orders o ON o.id=i.order_id WHERE i.node_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002' ORDER BY i.created_at;"
psql "$POSTGRES_DSN" -c "SELECT id,name,status,external_id,maintenance_mode,vf_enabled,vf_commissioned,capacity_instances,supported_tiers FROM vps.nodes WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002';"
