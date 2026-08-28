#!/usr/bin/env bash
# Finish DE-midrange setup @ 66.151.40.165
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
DE_MID_IP="${DE_MID_IP:-66.151.40.165}"
DE_MID_PASS="${DE_MID_PASS:?set DE_MID_PASS}"
DE_MID_HV_ID="${DE_MID_HV_ID:-5}"
DE_MID_GROUP="${DE_MID_GROUP:-5}"

set -a
source "$ROOT/infra/docker/.env.probe"
set +a

ssh-keygen -f /root/.ssh/known_hosts -R "$DE_MID_IP" 2>/dev/null || true
ssh-keyscan -H "$DE_MID_IP" >> /root/.ssh/known_hosts 2>/dev/null || true

echo "=== 1. Host: br0 + routed pool + storage ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 "root@${DE_MID_IP}" 'bash -s' <<'REMOTE'
set -euo pipefail
hostnamectl set-hostname DE-midrange
mkdir -p /home/vf-data/disk && chmod 755 /home/vf-data /home/vf-data/disk
NIC=$(ip -o link show | awk -F': ' '$2 !~ /^(lo|br|docker|veth|virbr)/ {print $2; exit}')
if ! ip addr show br0 2>/dev/null | grep -q "66.151.40.165"; then
  apt-get update -y
  apt-get install -y bridge-utils ifupdown2 curl sshpass 2>/dev/null || apt-get install -y bridge-utils curl
  cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback
auto ${NIC}
iface ${NIC} inet manual
auto br0
iface br0 inet static
  address 66.151.40.165
  netmask 255.255.255.192
  gateway 66.151.40.129
  bridge_ports ${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
up ip route replace 212.102.227.0/24 dev br0 scope link
up ip addr add 212.102.227.1/32 dev br0 || true
EOF
  ifdown "$NIC" 2>/dev/null || true
  ifdown br0 2>/dev/null || true
  ifup br0 || true
  sleep 3
fi
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 2>/dev/null || true
sysctl -w net.ipv4.conf.br0.proxy_arp=1 2>/dev/null || true
command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
if [ ! -d /opt/virtfusion/app/hypervisor ]; then
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
  sleep 8
fi
ls -la /dev/kvm 2>&1 || true
test -d /opt/virtfusion/app/hypervisor && echo "MID_HOST_OK"
REMOTE

echo "=== 2. VF NL: SSH key, storage, re-commission HV ${DE_MID_HV_ID} ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_MID_PASS}'
ssh-keygen -f /root/.ssh/known_hosts -R '${DE_MID_IP}' 2>/dev/null || true
sshpass -e ssh -o StrictHostKeyChecking=no root@${DE_MID_IP} \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=20 root@${DE_MID_IP} hostname

HV=\$("\${MYN[@]}" -e "SELECT id FROM hypervisors WHERE ip='${DE_MID_IP}' LIMIT 1;")
"\${MY[@]}" -e "UPDATE hypervisors SET name='DE-midrange', ip='${DE_MID_IP}', hypervisor_group_id=${DE_MID_GROUP}, enabled=1, commissioned=0 WHERE id=\$HV;"
ST=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=\$HV;")
if [[ "\$ST" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_storage (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES (\$HV,'Local disk','/home/vf-data/disk','mountpoint',8000,0,1,1,'[]',NOW(),NOW());"
fi
NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=\$HV AND \`primary\`=1 LIMIT 1;")
if [[ -z "\${NET:-}" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_networks (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
    VALUES (\$HV,'simpleBridge','br0',1,1,1,NOW(),NOW());"
  NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=\$HV AND \`primary\`=1 LIMIT 1;")
fi
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,\$HV);"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,\$NET,\$HV);"
for PKG in 17 18 19 20 21 10; do
  "\${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (${DE_MID_GROUP},\$PKG);" 2>/dev/null || true
done
cd /opt/virtfusion/app/control
printf "\${HV}\nyes\nyes\n" | \$PHP artisan hypervisor:re-commission 2>&1 | tail -10 || true
"\${MY[@]}" -e "SELECT id,name,ip,commissioned,hypervisor_group_id FROM hypervisors WHERE id=\$HV;"
REMOTE

echo "=== 3. Portal: DE-mid online ==="
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"
export POSTGRES_DSN
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes
SET status = 'online', vf_commissioned = 3, vf_enabled = true, updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

SELECT id, name, vf_name, vf_ip, external_id, supported_tiers, status
FROM vps.nodes WHERE region = 'de' ORDER BY name;

SELECT EXISTS (
  SELECT 1 FROM vps.nodes n WHERE n.region = 'de' AND n.status = 'online'
    AND 'midrange' = ANY(n.supported_tiers) AND NOT ('prosto' = ANY(n.supported_tiers))
) AS can_sell_de_midrange;
SQL

echo "DE_MIDRANGE_SETUP_COMPLETE"
