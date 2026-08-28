#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
DE_MID_PASS="$DE_MID_SSH_PASS"

echo "=== Fix DE-mid br0 (remove wrong 172.21.5, correct gw/mask) ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'MID'
set -euo pipefail
PRIMARY_IP=66.151.40.165
GW=66.151.40.129
NIC=$(ip -o link show | awk -F': ' '$2 !~ /^(lo|br|docker|veth|virbr)/ {print $2; exit}')
# Remove stray secondary on br0
ip addr del 172.21.5.189/24 dev br0 2>/dev/null || true
cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback
auto ${NIC}
iface ${NIC} inet manual
auto br0
iface br0 inet static
  address ${PRIMARY_IP}
  netmask 255.255.255.192
  gateway ${GW}
  bridge_ports ${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
up ip route replace 212.102.227.0/24 dev br0 scope link
up ip addr add 212.102.227.1/32 dev br0 || true
EOF
ifdown br0 2>/dev/null || true
ifup br0
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT
command -v ufw >/dev/null && ufw default allow routed 2>/dev/null && ufw reload 2>/dev/null || true
echo "br0:"; ip addr show br0 | grep inet
echo "routes:"; ip route | grep -E 'default|212.102'
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2 || true
MID

echo "=== Reset failed DE-mid orders ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL;"
NL

psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances SET external_id=NULL, ip_address=NULL, state='creating', updated_at=now()
WHERE id IN (SELECT i.id FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number IN (203,205));
SQL

cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1
echo "NETWORK_FIX_DONE"
