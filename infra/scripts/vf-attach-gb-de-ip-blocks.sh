#!/usr/bin/env bash
# Attach Hostkey /24 blocks to GB (UK) and DE hypervisors in VirtFusion.
# Run ON NL panel (66.248.206.14) as root.
set -euo pipefail

GB_HV_ID=4
GB_NET_ID=5
GB_GROUP_ID=4
GB_HV_IP=212.108.83.47
GB_BLK_NAME='GB public UK 91.108.247'
GB_POOL_GATEWAY='91.108.247.1'
GB_POOL_NETMASK='255.255.255.0'
GB_POOL_CIDR='91.108.247.0/24'
GB_POOL_START='91.108.247.2'
GB_POOL_END='91.108.247.254'

DE_HV_ID=3
DE_NET_ID=3
DE_GROUP_ID=3
DE_HV_IP=185.84.224.84
DE_BLK_NAME='DE public 212.102.227'
DE_POOL_GATEWAY='212.102.227.1'
DE_POOL_NETMASK='255.255.255.0'
DE_POOL_CIDR='212.102.227.0/24'
DE_POOL_START='212.102.227.2'
DE_POOL_END='212.102.227.254'

: "${GB_SSH_PASS:?set GB_SSH_PASS}"
: "${DE_SSH_PASS:?set DE_SSH_PASS}"

set -a
source /opt/virtfusion/app/control/.env
set +a

PHP="${PHP:-/opt/virtfusion/php8/bin/php}"
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

ensure_nl_ssh() {
  local ip=$1 pass=$2
  if [ ! -f /root/.ssh/id_ed25519 ]; then
    ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
  fi
  local pub
  pub=$(cat /root/.ssh/id_ed25519.pub)
  ssh-keyscan -H "$ip" >> /root/.ssh/known_hosts 2>/dev/null || true
  export SSHPASS="$pass"
  sshpass -e ssh -o StrictHostKeyChecking=no "root@${ip}" \
    "grep -qxF '$pub' /root/.ssh/authorized_keys 2>/dev/null || echo '$pub' >> /root/.ssh/authorized_keys"
  ssh -o BatchMode=yes -o ConnectTimeout=15 "root@${ip}" hostname
}

setup_host_routed_pool() {
  local ip=$1 pass=$2 cidr=$3 gw=$4
  export SSHPASS="$pass"
  sshpass -e ssh -o StrictHostKeyChecking=no "root@${ip}" "bash -s" <<REMOTE
set -euo pipefail
CIDR='$cidr'
GW='$gw'
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.proxy_arp=1
sysctl -w net.ipv4.conf.br0.proxy_arp=1
# Routed /24: link route + gateway /32 on br0 (guests ARP the GW).
ip route replace \${CIDR} dev br0 scope link || ip route add \${CIDR} dev br0 scope link
ip addr show dev br0 | grep -q "\${GW}/" || ip addr add \${GW}/32 dev br0
# UFW blocks forwarded guest traffic unless routed policy is allow.
command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
IF=/etc/network/interfaces
if [ -f "\$IF" ] && ! grep -q "vf-routed-\${GW}" "\$IF"; then
  cat >>"\$IF" <<IFACE

# vf-routed-\${GW}
auto br0:vf-\${GW//./-}
iface br0:vf-\${GW//./-} inet static
    address \${GW}
    netmask 255.255.255.255
    post-up ip route replace \${CIDR} dev br0 scope link || true
    pre-down ip addr del \${GW}/32 dev br0 || true
IFACE
fi
ip route | grep "\${CIDR%%/*}" || true
ip -br addr show br0
sysctl net.ipv4.ip_forward net.ipv4.conf.br0.proxy_arp
REMOTE
}

attach_ip_block() {
  local hv_id=$1 net_id=$2 blk_name=$3 gateway=$4 netmask=$5 start_ip=$6 end_ip=$7 old_block_id=${8:-}

  echo "=== IP block: $blk_name (HV $hv_id) ==="
  local blk_id
  blk_id=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='$blk_name' LIMIT 1;" || true)
  if [[ -z "${blk_id:-}" ]]; then
    "${MY[@]}" -e "INSERT INTO ip_blocks
      (type,name,ipv4_gateway,ipv4_netmask,ipv4_resolver_1,ipv4_resolver_2,enabled,rdns_type,network_profile,dhcp,created_at,updated_at)
      VALUES (4,'$blk_name','$gateway','$netmask','1.1.1.1','8.8.8.8',1,0,0,1,NOW(),NOW());"
    blk_id=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='$blk_name' LIMIT 1;")
  else
    "${MY[@]}" -e "UPDATE ip_blocks SET ipv4_gateway='$gateway', ipv4_netmask='$netmask', enabled=1 WHERE id=$blk_id;"
  fi
  echo "block_id=$blk_id"

  if [[ -n "${old_block_id:-}" && "$old_block_id" != "$blk_id" ]]; then
    echo "Disable old block $old_block_id links for HV $hv_id"
    "${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE block_id=$old_block_id AND hypervisor_id=$hv_id;"
    "${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE block_id=$old_block_id AND hypervisor_id=$hv_id;"
    "${MY[@]}" -e "UPDATE ip_blocks SET enabled=0 WHERE id=$old_block_id;"
  fi

  "${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE block_id=$blk_id AND hypervisor_id=$hv_id;"
  "${MY[@]}" -e "INSERT INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES ($blk_id,$hv_id);"
  "${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE block_id=$blk_id AND network_id=$net_id AND hypervisor_id=$hv_id;"
  "${MY[@]}" -e "INSERT INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES ($blk_id,$net_id,$hv_id);"

  python3 - <<PY
import ipaddress
start = ipaddress.ip_address("$start_ip")
end = ipaddress.ip_address("$end_ip")
cur = int(start)
end_i = int(end)
lines = []
while cur <= end_i:
    lines.append(str(ipaddress.ip_address(cur)))
    cur += 1
open("/tmp/vf-pool-ips.txt","w").write("\n".join(lines))
print("pool_ips", len(lines))
PY

  local n=0
  while IFS= read -r IP; do
    [[ -z "$IP" ]] && continue
    cat >/tmp/vf-pool-one.sql <<EOSQL
INSERT IGNORE INTO ipv4 (address,server_id,block_id,reserved,\`order\`,created_at,updated_at)
VALUES (INET_ATON('$IP'),NULL,$blk_id,0,1,NOW(),NOW());
EOSQL
    "${MY[@]}" < /tmp/vf-pool-one.sql
    n=$((n+1))
  done < /tmp/vf-pool-ips.txt
  echo "inserted/ignored $n rows for block $blk_id"

  local free
  free=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ipv4 WHERE block_id=$blk_id AND server_id IS NULL AND (reserved IS NULL OR reserved=0);")
  echo "free_ips=$free"
}

echo "=== NL -> GB/DE SSH ==="
ensure_nl_ssh "$GB_HV_IP" "$GB_SSH_PASS"
ensure_nl_ssh "$DE_HV_IP" "$DE_SSH_PASS"

echo "=== Host routed pools ==="
setup_host_routed_pool "$GB_HV_IP" "$GB_SSH_PASS" "$GB_POOL_CIDR" "$GB_POOL_GATEWAY"
setup_host_routed_pool "$DE_HV_IP" "$DE_SSH_PASS" "$DE_POOL_CIDR" "$DE_POOL_GATEWAY"

OLD_GB=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='GB public UK' LIMIT 1;" || true)
attach_ip_block "$GB_HV_ID" "$GB_NET_ID" "$GB_BLK_NAME" "$GB_POOL_GATEWAY" "$GB_POOL_NETMASK" "$GB_POOL_START" "$GB_POOL_END" "${OLD_GB:-}"

attach_ip_block "$DE_HV_ID" "$DE_NET_ID" "$DE_BLK_NAME" "$DE_POOL_GATEWAY" "$DE_POOL_NETMASK" "$DE_POOL_START" "$DE_POOL_END"

echo "=== re-commission GB + DE ==="
cd /opt/virtfusion/app/control
for HV in $GB_HV_ID $DE_HV_ID; do
  echo "commission HV $HV"
  printf "${HV}\nyes\nyes\n" | $PHP artisan hypervisor:re-commission 2>&1 | tail -8 || true
done

echo "=== verify ==="
"${MY[@]}" -e "SELECT ib.id,ib.name,ib.ipv4_gateway,ib.enabled,COUNT(i.id) total,
  SUM(CASE WHEN i.server_id IS NULL AND (i.reserved IS NULL OR i.reserved=0) THEN 1 ELSE 0 END) free
  FROM ip_blocks ib LEFT JOIN ipv4 i ON i.block_id=ib.id
  WHERE ib.id IN (SELECT block_id FROM ip_block_hypervisor WHERE hypervisor_id IN (3,4))
  GROUP BY ib.id ORDER BY ib.id;"
"${MY[@]}" -e "SELECT ibh.hypervisor_id, ib.id, ib.name FROM ip_block_hypervisor ibh JOIN ip_blocks ib ON ib.id=ibh.block_id WHERE ibh.hypervisor_id IN (3,4) ORDER BY ibh.hypervisor_id;"
echo VF_ATTACH_GB_DE_IP_DONE
