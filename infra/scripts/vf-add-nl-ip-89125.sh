#!/usr/bin/env bash
# Attach 89.125.42.0/24 to NL hypervisor (VirtFusion + routed pool on br0).
# Run ON NL panel / HV (66.248.206.14) as root.
set -euo pipefail

HV_ID=1
NET_ID=1
BLK_NAME='NL public 89.125.42'
POOL_GATEWAY='89.125.42.1'
POOL_NETMASK='255.255.255.0'
POOL_CIDR='89.125.42.0/24'
POOL_START='89.125.42.2'
POOL_END='89.125.42.254'

set -a
source /opt/virtfusion/app/control/.env
set +a

PHP="${PHP:-/opt/virtfusion/php8/bin/php}"
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

setup_routed_pool() {
  local cidr=$1 gw=$2
  echo "=== Host routed pool $cidr gw=$gw ==="
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
  ip route replace "${cidr}" dev br0 scope link || ip route add "${cidr}" dev br0 scope link
  ip addr show dev br0 | grep -q "${gw}/" || ip addr add "${gw}/32" dev br0
  command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
  iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
  IF=/etc/network/interfaces
  if [ -f "$IF" ] && ! grep -q "vf-routed-${gw}" "$IF"; then
    cat >>"$IF" <<IFACE

# vf-routed-${gw} (${cidr})
auto br0:vf-${gw//./-}
iface br0:vf-${gw//./-} inet static
    address ${gw}
    netmask 255.255.255.255
    post-up ip route replace ${cidr} dev br0 scope link || true
    pre-down ip addr del ${gw}/32 dev br0 || true
IFACE
  fi
  ip route | grep "${cidr%%/*}" || true
  ip -br addr show br0
}

attach_ip_block() {
  echo "=== VF ip block: $BLK_NAME (HV $HV_ID) ==="
  local blk_id
  blk_id=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='$BLK_NAME' LIMIT 1;" || true)
  if [[ -z "${blk_id:-}" ]]; then
    "${MY[@]}" -e "INSERT INTO ip_blocks
      (type,name,ipv4_gateway,ipv4_netmask,ipv4_resolver_1,ipv4_resolver_2,enabled,rdns_type,network_profile,dhcp,created_at,updated_at)
      VALUES (4,'$BLK_NAME','$POOL_GATEWAY','$POOL_NETMASK','1.1.1.1','8.8.8.8',1,0,0,1,NOW(),NOW());"
    blk_id=$("${MYN[@]}" -e "SELECT id FROM ip_blocks WHERE name='$BLK_NAME' LIMIT 1;")
  else
    "${MY[@]}" -e "UPDATE ip_blocks SET ipv4_gateway='$POOL_GATEWAY', ipv4_netmask='$POOL_NETMASK', enabled=1 WHERE id=$blk_id;"
  fi
  echo "block_id=$blk_id"

  "${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE block_id=$blk_id AND hypervisor_id=$HV_ID;"
  "${MY[@]}" -e "INSERT INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES ($blk_id,$HV_ID);"
  "${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE block_id=$blk_id AND network_id=$NET_ID AND hypervisor_id=$HV_ID;"
  "${MY[@]}" -e "INSERT INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES ($blk_id,$NET_ID,$HV_ID);"

  python3 - <<PY
import ipaddress
start = ipaddress.ip_address("$POOL_START")
end = ipaddress.ip_address("$POOL_END")
cur = int(start)
end_i = int(end)
lines = []
while cur <= end_i:
    lines.append(str(ipaddress.ip_address(cur)))
    cur += 1
open("/tmp/vf-nl-pool-ips.txt","w").write("\n".join(lines))
print("pool_ips", len(lines))
PY

  local n=0
  while IFS= read -r IP; do
    [[ -z "$IP" ]] && continue
    cat >/tmp/vf-nl-pool-one.sql <<EOSQL
INSERT IGNORE INTO ipv4 (address,server_id,block_id,reserved,\`order\`,created_at,updated_at)
VALUES (INET_ATON('$IP'),NULL,$blk_id,0,1,NOW(),NOW());
EOSQL
    "${MY[@]}" < /tmp/vf-nl-pool-one.sql
    n=$((n+1))
  done < /tmp/vf-nl-pool-ips.txt
  echo "inserted/ignored $n rows for block $blk_id"

  local free total
  free=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ipv4 WHERE block_id=$blk_id AND server_id IS NULL AND (reserved IS NULL OR reserved=0);")
  total=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ipv4 WHERE block_id=$blk_id;")
  echo "free_ips=$free total_ips=$total"
}

setup_routed_pool "$POOL_CIDR" "$POOL_GATEWAY"
attach_ip_block

echo "=== re-commission NL HV $HV_ID ==="
cd /opt/virtfusion/app/control
printf "${HV_ID}\nyes\nyes\n" | $PHP artisan hypervisor:re-commission 2>&1 | tail -12 || true

echo "=== verify ==="
"${MY[@]}" -e "SELECT ib.id,ib.name,ib.ipv4_gateway,ib.enabled,COUNT(i.id) total,
  SUM(CASE WHEN i.server_id IS NULL AND (i.reserved IS NULL OR i.reserved=0) THEN 1 ELSE 0 END) free
  FROM ip_blocks ib
  JOIN ip_block_hypervisor ibh ON ibh.block_id=ib.id AND ibh.hypervisor_id=$HV_ID
  LEFT JOIN ipv4 i ON i.block_id=ib.id
  GROUP BY ib.id ORDER BY ib.id;"
echo VF_ADD_NL_IP_89125_DONE
