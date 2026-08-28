#!/bin/bash
# Add Hetzner additional IP 95.216.1.146 to FI hypervisor (VirtFusion + routed libvirt).
# Same /26 as primary (gw 95.216.1.129) — separate ip_block from 135.181.123.x pool.
set -euo pipefail

FI_IP="${FI_IP:-95.216.1.155}"
FI_SSH_PASS="${FI_SSH_PASS:-EidNq_riB9F3rD}"
NL_SSH_PASS="${NL_SSH_PASS:-zx_zvJdI9P}"

NEW_IP="${NEW_IP:-95.216.1.146}"
GW="${GW:-95.216.1.129}"
NETMASK="${NETMASK:-255.255.255.192}"
BLK_NAME="${BLK_NAME:-FI public HEL /26}"
HV_ID="${HV_ID:-2}"
NET_ID="${NET_ID:-2}"

run_fi() {
  SSHPASS="$FI_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "root@${FI_IP}" "$@"
}

run_nl() {
  SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@66.248.206.14 "$@"
}

echo "=== VF DB: ensure ip block + ipv4 row for ${NEW_IP} ==="
run_nl "bash -s" <<REMOTE
set -euo pipefail
source /opt/virtfusion/app/control/.env
MY=(mysql -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(mysql -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)

echo "--- current FI blocks ---"
"\${MY[@]}" -e "SELECT id,name,ipv4_gateway,ipv4_netmask FROM ip_blocks WHERE name LIKE 'FI%';"
echo "--- current ${NEW_IP} ---"
"\${MY[@]}" -e "SELECT id,INET_NTOA(address) ip,block_id,server_id FROM ipv4 WHERE address=INET_ATON('${NEW_IP}');"

BLK_ID=\$("\${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='${BLK_NAME}' LIMIT 1;" || true)
if [[ -z "\${BLK_ID:-}" ]]; then
  "\${MY[@]}" -e "INSERT INTO ip_blocks
    (type,name,ipv4_gateway,ipv4_netmask,ipv4_resolver_1,ipv4_resolver_2,enabled,rdns_type,network_profile,dhcp,created_at,updated_at)
    VALUES (4,'${BLK_NAME}','${GW}','${NETMASK}','185.12.64.1','185.12.64.2',1,0,0,1,NOW(),NOW());"
  BLK_ID=\$("\${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='${BLK_NAME}' LIMIT 1;")
  echo "created block id=\$BLK_ID"
else
  "\${MY[@]}" -e "UPDATE ip_blocks SET ipv4_gateway='${GW}', ipv4_netmask='${NETMASK}', enabled=1 WHERE id=\$BLK_ID;"
  echo "updated block id=\$BLK_ID"
fi

"\${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE block_id=\$BLK_ID AND hypervisor_id=${HV_ID};"
"\${MY[@]}" -e "INSERT INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (\$BLK_ID,${HV_ID});"
"\${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE block_id=\$BLK_ID AND network_id=${NET_ID} AND hypervisor_id=${HV_ID};"
"\${MY[@]}" -e "INSERT INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (\$BLK_ID,${NET_ID},${HV_ID});"

EXISTS=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM ipv4 WHERE address=INET_ATON('${NEW_IP}');")
if [[ "\$EXISTS" == "0" ]]; then
  cat >/tmp/vf-add-ipv4.sql <<'EOSQL'
INSERT INTO ipv4 (address,server_id,block_id,reserved,\`order\`,created_at,updated_at)
VALUES (INET_ATON('__NEW_IP__'),NULL,__BLK_ID__,0,1,NOW(),NOW());
EOSQL
  sed -i "s/__NEW_IP__/${NEW_IP}/g; s/__BLK_ID__/\$BLK_ID/g" /tmp/vf-add-ipv4.sql
  "\${MY[@]}" < /tmp/vf-add-ipv4.sql
  echo "inserted ipv4 ${NEW_IP}"
else
  "\${MY[@]}" -e "UPDATE ipv4 SET block_id=\$BLK_ID, reserved=0 WHERE address=INET_ATON('${NEW_IP}');"
  echo "updated ipv4 ${NEW_IP} -> block \$BLK_ID"
fi

echo "--- after ---"
"\${MY[@]}" -e "SELECT id,name,ipv4_gateway,ipv4_netmask FROM ip_blocks WHERE id=\$BLK_ID;"
"\${MY[@]}" -e "SELECT id,INET_NTOA(address) ip,block_id,server_id,reserved FROM ipv4 WHERE block_id=\$BLK_ID ORDER BY address;"
REMOTE

echo "=== FI HV: persistent route ${NEW_IP}/32 -> br0 (Hetzner routed additional IP) ==="
run_fi "bash -s" <<REMOTE
set -euo pipefail
ROUTE="${NEW_IP}/32"
IFACE=/etc/network/interfaces

if ! grep -q "${NEW_IP}/32 dev br0" "\$IFACE"; then
  cp -a "\$IFACE" "\${IFACE}.bak.add-${NEW_IP}"
  sed -i "/post-up ip route add 135.181.123.152\\/29 dev br0/a\\    post-up ip route add ${NEW_IP}/32 dev br0 || true" "\$IFACE"
  sed -i "/pre-down ip route del 135.181.123.152\\/29 dev br0/a\\    pre-down ip route del ${NEW_IP}/32 dev br0 || true" "\$IFACE"
  echo "updated \$IFACE"
fi

ip route add ${NEW_IP}/32 dev br0 2>/dev/null || true

echo "--- verify ---"
ip -br addr
ip route | grep -E '135.181|95.216.1.146' || true
sysctl net.ipv4.conf.all.proxy_arp net.ipv4.conf.eth0.proxy_arp 2>/dev/null || true
virsh net-list --all
REMOTE

echo VF_ADD_FI_IP_${NEW_IP//./_}_DONE
