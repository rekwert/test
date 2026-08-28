#!/bin/bash
# Restore GB hypervisor row in VF DB (run on NL panel 66.248.206.14).
set -euo pipefail
source /opt/virtfusion/app/control/.env

GB_HV_ID=4
GB_NET_ID=4
GB_GROUP_ID=4
GB_HV_IP="${GB_HV_IP:-212.108.83.47}"
GB_SSH_PASS="${GB_SSH_PASS:?set GB_SSH_PASS}"
BK="/root/gb-hv-backup-20260809.sql"

MY=(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

TOKEN=""
if [[ -f "$BK" ]]; then
  TOKEN=$(python3 - <<PY
import re
text = open("$BK", encoding="utf-8", errors="replace").read()
m = re.search(r"token: (\S+)", text)
print(m.group(1) if m else "")
PY
)
fi
[[ -n "$TOKEN" ]] || { echo "GB token not found in $BK"; exit 1; }

EXISTS=$("${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisors WHERE id=$GB_HV_ID;")
if [[ "$EXISTS" == "0" ]]; then
  echo "inserting hypervisor id=$GB_HV_ID"
  "${MY[@]}" <<SQL
INSERT INTO hypervisors
  (id,type,arch,commissioned,name,ip,port,ssh_port,hypervisor_group_id,enabled,prohibit,nf_type,
   max_servers,max_cpu,max_memory,max_local_hdd,max_local_hdd_enabled,local_hdd_storage_type,
   maintenance,disk_type,disk_cache_type,backup_storage_type,default_cpu,default_machine_type,
   token,license_type,created_at,updated_at)
VALUES
  ($GB_HV_ID,1,1,0,'GB-prosto','$GB_HV_IP',8892,22,$GB_GROUP_ID,1,0,4,
   50,96,255772,1000,1,0,0,'inherit','inherit',2,'inherit','inherit',
   '$TOKEN',1,NOW(),NOW());
SQL
else
  echo "updating existing hypervisor id=$GB_HV_ID"
  "${MY[@]}" -e "UPDATE hypervisors SET
    name='GB-prosto', ip='$GB_HV_IP', hypervisor_group_id=$GB_GROUP_ID,
    enabled=1, prohibit=0, maintenance=0, port=8892, ssh_port=22,
    token='$TOKEN', updated_at=NOW()
    WHERE id=$GB_HV_ID;"
fi

NET=$("${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_networks WHERE hypervisor_id=$GB_HV_ID AND \`primary\`=1;")
if [[ "$NET" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO hypervisor_networks
    (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
    VALUES ($GB_HV_ID,'simpleBridge','br0',1,1,1,NOW(),NOW());"
fi

ST=$("${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=$GB_HV_ID;")
if [[ "$ST" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO hypervisor_storage
    (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES ($GB_HV_ID,'Local disk','/home/vf-data/disk','mountpoint',2000,0,1,1,'[]',NOW(),NOW());"
fi

"${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE block_id=4 AND hypervisor_id=$GB_HV_ID;"
"${MY[@]}" -e "INSERT INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (4,$GB_HV_ID);"
"${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE block_id=4 AND network_id=$GB_NET_ID AND hypervisor_id=$GB_HV_ID;"
"${MY[@]}" -e "INSERT INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (4,$GB_NET_ID,$GB_HV_ID);"

GRP=$("${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_groups WHERE id=$GB_GROUP_ID;")
if [[ "$GRP" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO hypervisor_groups
    (id,name,description,\`default\`,distribution_type,label,type,icon,visible,visible_label,enabled,created_at,updated_at)
    VALUES ($GB_GROUP_ID,'GB','United Kingdom London',0,5,NULL,NULL,NULL,0,1,1,NOW(),NOW());"
else
  "${MY[@]}" -e "UPDATE hypervisor_groups SET name='GB', enabled=1 WHERE id=$GB_GROUP_ID;"
fi

for PKG in $("${MYN[@]}" -e "SELECT id FROM server_packages WHERE enabled=1;"); do
  "${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
    VALUES ($GB_GROUP_ID,$PKG);" 2>/dev/null || true
done

echo "=== NL SSH -> GB ==="
if [ ! -f /root/.ssh/id_ed25519 ]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(cat /root/.ssh/id_ed25519.pub)
ssh-keyscan -H "$GB_HV_IP" >> /root/.ssh/known_hosts 2>/dev/null || true
export SSHPASS="$GB_SSH_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no "root@${GB_HV_IP}" \
  "grep -qxF '$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 "root@${GB_HV_IP}" hostname

echo "=== re-commission GB ==="
cd /opt/virtfusion/app/control
/opt/virtfusion/php8/bin/php artisan hypervisor:re-commission --no-interaction 2>&1 | tail -20 || true

echo "=== verify ==="
"${MY[@]}" -e "SELECT id,name,ip,enabled,prohibit,maintenance,commissioned,hypervisor_group_id FROM hypervisors ORDER BY id;"
"${MY[@]}" -e "SELECT COUNT(*) AS free_ips FROM ipv4 WHERE block_id=4 AND server_id IS NULL;"
echo VF_GB_RESTORED
