#!/bin/bash
# VF DB + IP pool for GB hypervisor. Run ON NL panel (66.248.206.14).
set -euo pipefail
source /opt/virtfusion/app/control/.env

GB_HV_ID=4
GB_NET_ID=4
GB_GROUP_ID=4
GB_HV_IP=212.108.83.47
GB_BLK_NAME='GB public UK'
GB_POOL_GATEWAY='172.24.5.1'
GB_POOL_NETMASK='255.255.255.0'
GB_POOL_START='172.24.5.159'
GB_POOL_END='172.24.5.174'

MY=(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

echo "=== SSH NL -> GB ==="
if [ ! -f /root/.ssh/id_ed25519 ]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(cat /root/.ssh/id_ed25519.pub)
ssh-keyscan -H "$GB_HV_IP" >> /root/.ssh/known_hosts 2>/dev/null || true
export SSHPASS="${GB_SSH_PASS:?set GB_SSH_PASS}"
sshpass -e ssh -o StrictHostKeyChecking=no "root@${GB_HV_IP}" \
  "grep -qxF '$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 "root@${GB_HV_IP}" hostname

echo "=== hypervisor group $GB_GROUP_ID ==="
GRP=$("${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_groups WHERE id=$GB_GROUP_ID;")
if [[ "$GRP" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO hypervisor_groups
    (id,name,description,\`default\`,distribution_type,label,type,icon,visible,visible_label,enabled,created_at,updated_at)
    VALUES ($GB_GROUP_ID,'GB','United Kingdom London',0,5,NULL,NULL,NULL,0,1,1,NOW(),NOW());"
else
  "${MY[@]}" -e "UPDATE hypervisor_groups SET name='GB', enabled=1 WHERE id=$GB_GROUP_ID;"
fi
"${MY[@]}" -e "UPDATE hypervisors SET hypervisor_group_id=$GB_GROUP_ID, name='GB-prosto', enabled=1 WHERE id=$GB_HV_ID;"

for PKG in $("${MYN[@]}" -e "SELECT id FROM server_packages WHERE enabled=1;"); do
  "${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
    VALUES ($GB_GROUP_ID,$PKG);" 2>/dev/null || true
done

echo "=== IP block ==="
BLK_ID=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='$GB_BLK_NAME' LIMIT 1;" || true)
if [[ -z "${BLK_ID:-}" ]]; then
  "${MY[@]}" -e "INSERT INTO ip_blocks
    (type,name,ipv4_gateway,ipv4_netmask,ipv4_resolver_1,ipv4_resolver_2,enabled,rdns_type,network_profile,dhcp,created_at,updated_at)
    VALUES (4,'$GB_BLK_NAME','$GB_POOL_GATEWAY','$GB_POOL_NETMASK','1.1.1.1','8.8.8.8',1,0,0,1,NOW(),NOW());"
  BLK_ID=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='$GB_BLK_NAME' LIMIT 1;")
else
  "${MY[@]}" -e "UPDATE ip_blocks SET ipv4_gateway='$GB_POOL_GATEWAY', ipv4_netmask='$GB_POOL_NETMASK', enabled=1 WHERE id=$BLK_ID;"
fi
"${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE block_id=$BLK_ID AND hypervisor_id=$GB_HV_ID;"
"${MY[@]}" -e "INSERT INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES ($BLK_ID,$GB_HV_ID);"
"${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE block_id=$BLK_ID AND network_id=$GB_NET_ID AND hypervisor_id=$GB_HV_ID;"
"${MY[@]}" -e "INSERT INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES ($BLK_ID,$GB_NET_ID,$GB_HV_ID);"

python3 - <<PY
import ipaddress
start = ipaddress.ip_address("$GB_POOL_START")
end = ipaddress.ip_address("$GB_POOL_END")
skip = {"172.24.5.158"}
cur = int(start)
end_i = int(end)
lines = []
while cur <= end_i:
    ip = str(ipaddress.ip_address(cur))
    if ip not in skip:
        lines.append(ip)
    cur += 1
open("/tmp/vf-gb-ips.txt","w").write("\n".join(lines))
print("ips", len(lines))
PY

while IFS= read -r IP; do
  [[ -z "$IP" ]] && continue
  cat >/tmp/vf-gb-one.sql <<EOSQL
INSERT IGNORE INTO ipv4 (address,server_id,block_id,reserved,\`order\`,created_at,updated_at)
VALUES (INET_ATON('$IP'),NULL,$BLK_ID,0,1,NOW(),NOW());
EOSQL
  "${MY[@]}" < /tmp/vf-gb-one.sql
done < /tmp/vf-gb-ips.txt

echo "=== re-commission ==="
cd /opt/virtfusion/app/control
/opt/virtfusion/php8/bin/php artisan hypervisor:re-commission --no-interaction 2>&1 | tail -15 || true

echo "=== verify ==="
"${MY[@]}" -e "SELECT id,name,ip,hypervisor_group_id,commissioned FROM hypervisors ORDER BY id;"
"${MY[@]}" -e "SELECT id,name FROM hypervisor_groups ORDER BY id;"
"${MY[@]}" -e "SELECT INET_NTOA(address) ip, block_id, server_id FROM ipv4 WHERE block_id=$BLK_ID ORDER BY address;"
echo VF_GB_VF_DB_DONE
