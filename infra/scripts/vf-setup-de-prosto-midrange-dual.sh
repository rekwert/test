#!/usr/bin/env bash
# DE dual nodes: prosto @ 185.84.224.84, midrange @ 66.151.40.165, shared IP pool 212.102.227.0/24
# Run on back server. Passwords: DE_SSH_PASS (prosto), DE_MID_SSH_PASS (midrange, defaults to DE_SSH_PASS).
set -euo pipefail

ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env.probe"
set +a

DE_PROSTO_IP="${DE_PROSTO_IP:-185.84.224.84}"
DE_PROSTO_GW="${DE_PROSTO_GW:-185.84.224.65}"
DE_PROSTO_MASK="${DE_PROSTO_MASK:-255.255.255.192}"
DE_PROSTO_HV_ID="${DE_PROSTO_HV_ID:-3}"
DE_PROSTO_GROUP="${DE_PROSTO_GROUP:-3}"

DE_MID_IP="${DE_MID_IP:-66.151.40.165}"
DE_MID_GW="${DE_MID_GW:-66.151.40.129}"
DE_MID_MASK="${DE_MID_MASK:-255.255.255.192}"
DE_MID_HV_ID="${DE_MID_HV_ID:-5}"
DE_MID_GROUP="${DE_MID_GROUP:-5}"

DE_MID_SSH_PASS="${DE_MID_SSH_PASS:-$DE_SSH_PASS}"
DE_BLOCK_ID="${DE_BLOCK_ID:-6}"
DE_POOL_CIDR="${DE_POOL_CIDR:-212.102.227.0/24}"
DE_POOL_GW="${DE_POOL_GW:-212.102.227.1}"

setup_host() {
  local ip=$1 pass=$2 gw=$3 mask=$4 addr=$5 hostname=$6
  SSHPASS="$pass" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 "root@${ip}" \
    "DE_ADDR=${addr} DE_GW=${gw} DE_MASK=${mask} DE_HOSTNAME=${hostname} bash -s" <<'REMOTE'
set -euo pipefail
hostnamectl set-hostname "$DE_HOSTNAME"
mkdir -p /home/vf-data/disk && chmod 755 /home/vf-data /home/vf-data/disk
PRIMARY_IP="$DE_ADDR"
GW="$DE_GW"
MASK="$DE_MASK"
NIC=$(ip -o -4 route show default | awk '{print $5; exit}')
[[ -z "$NIC" || "$NIC" == "br0" ]] && NIC=$(ip -o link show | awk -F': ' '$2 !~ /^(lo|br|docker|veth|virbr)/ {print $2; exit}')
if ip addr show br0 2>/dev/null | grep -q "$PRIMARY_IP"; then
  echo "br0 already has $PRIMARY_IP"
else
  apt-get update -y
  apt-get install -y bridge-utils curl ifupdown2 sshpass 2>/dev/null || apt-get install -y bridge-utils curl
  cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback
auto ${NIC}
iface ${NIC} inet manual
auto br0
iface br0 inet static
  address ${PRIMARY_IP}
  netmask ${MASK}
  gateway ${GW}
  bridge_ports ${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
EOF
  ifdown "$NIC" 2>/dev/null || true
  ifdown br0 2>/dev/null || true
  ifup br0 || true
  sleep 2
fi
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
CIDR='212.102.227.0/24'
POOL_GW='212.102.227.1'
ip route replace ${CIDR} dev br0 scope link || true
ip addr show dev br0 | grep -q "${POOL_GW}/" || ip addr add ${POOL_GW}/32 dev br0
command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
if ! grep -q vf-routed-pool /etc/network/interfaces 2>/dev/null; then
  cat >>/etc/network/interfaces <<IFACE

# vf-routed-pool
up ip route replace ${CIDR} dev br0 scope link
up ip addr add ${POOL_GW}/32 dev br0 || true
IFACE
fi
if [ ! -d /opt/virtfusion/app/hypervisor ]; then
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
  sleep 8
fi
test -d /opt/virtfusion/app/hypervisor && echo "HOST_OK ${PRIMARY_IP}" || exit 1
REMOTE
}

echo "=== 1. Host prep: DE-prosto ${DE_PROSTO_IP} ==="
setup_host "$DE_PROSTO_IP" "$DE_SSH_PASS" "$DE_PROSTO_GW" "$DE_PROSTO_MASK" "$DE_PROSTO_IP" "DE-prosto"

echo "=== 2. Host prep: DE-midrange ${DE_MID_IP} ==="
setup_host "$DE_MID_IP" "$DE_MID_SSH_PASS" "$DE_MID_GW" "$DE_MID_MASK" "$DE_MID_IP" "DE-midrange"

echo "=== 3. VirtFusion NL: prosto restore + midrange HV + shared pool ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 "bash -s" <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)

ensure_key() {
  local ip=\$1 pass=\$2
  if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
  local pub=\$(cat /root/.ssh/id_ed25519.pub)
  export SSHPASS="\$pass"
  sshpass -e ssh -o StrictHostKeyChecking=no root@\${ip} \
    "grep -qxF '\$pub' /root/.ssh/authorized_keys 2>/dev/null || echo '\$pub' >> /root/.ssh/authorized_keys"
  ssh -o BatchMode=yes -o ConnectTimeout=15 root@\${ip} hostname
}

ensure_key ${DE_PROSTO_IP} '${DE_SSH_PASS}'
ensure_key ${DE_MID_IP} '${DE_MID_SSH_PASS}'

"\${MY[@]}" -e "UPDATE hypervisors SET name='DE-prosto', ip='${DE_PROSTO_IP}', hypervisor_group_id=${DE_PROSTO_GROUP}, enabled=1, commissioned=0 WHERE id=${DE_PROSTO_HV_ID};"
"\${MY[@]}" -e "UPDATE hypervisor_groups SET name='DE Prosto', enabled=1 WHERE id=${DE_PROSTO_GROUP};"

HV_MID=\$("\${MYN[@]}" -e "SELECT id FROM hypervisors WHERE ip='${DE_MID_IP}' LIMIT 1;" || true)
if [[ -z "\${HV_MID:-}" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisors
    (type,arch,commissioned,name,ip,port,ssh_port,hypervisor_group_id,enabled,prohibit,nf_type,
     max_servers,max_cpu,max_memory,max_local_hdd,max_local_hdd_enabled,local_hdd_storage_type,
     maintenance,disk_type,disk_cache_type,backup_storage_type,default_cpu,default_machine_type,created_at,updated_at)
    VALUES (1,1,0,'DE-midrange','${DE_MID_IP}',8892,22,${DE_MID_GROUP},1,0,4,
            80,128,262144,8000,1,0,0,'inherit','inherit',2,'inherit','inherit',NOW(),NOW());"
  HV_MID=\$("\${MYN[@]}" -e "SELECT id FROM hypervisors WHERE ip='${DE_MID_IP}' LIMIT 1;")
  echo "created mid HV id=\$HV_MID"
else
  "\${MY[@]}" -e "UPDATE hypervisors SET name='DE-midrange', enabled=1, commissioned=0, hypervisor_group_id=${DE_MID_GROUP},
    max_servers=80, max_cpu=128, max_memory=262144, max_local_hdd=8000 WHERE id=\$HV_MID;"
  echo "updated mid HV id=\$HV_MID"
fi

GRP=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_groups WHERE id=${DE_MID_GROUP};")
if [[ "\$GRP" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_groups
    (id,name,description,\`default\`,distribution_type,label,type,icon,visible,visible_label,enabled,created_at,updated_at)
    VALUES (${DE_MID_GROUP},'DE Midrange','Germany midrange dedicated',0,5,NULL,NULL,NULL,0,1,1,NOW(),NOW());"
else
  "\${MY[@]}" -e "UPDATE hypervisor_groups SET name='DE Midrange', enabled=1 WHERE id=${DE_MID_GROUP};"
fi
"\${MY[@]}" -e "UPDATE hypervisors SET hypervisor_group_id=${DE_MID_GROUP} WHERE id=\$HV_MID;"

for NET_HV in ${DE_PROSTO_HV_ID} \$HV_MID; do
  NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=\$NET_HV AND \`primary\`=1 LIMIT 1;" || true)
  if [[ -z "\${NET:-}" ]]; then
    "\${MY[@]}" -e "INSERT INTO hypervisor_networks (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
      VALUES (\$NET_HV,'simpleBridge','br0',1,1,1,NOW(),NOW());"
  else
    "\${MY[@]}" -e "UPDATE hypervisor_networks SET bridge='br0', enabled=1 WHERE id=\$NET;"
  fi
  ST=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=\$NET_HV;")
  if [[ "\$ST" == "0" ]]; then
    "\${MY[@]}" -e "INSERT INTO hypervisor_storage
      (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
      VALUES (\$NET_HV,'Local disk','/home/vf-data/disk','mountpoint',8000,0,1,1,'[]',NOW(),NOW());"
  fi
done

# Prosto packages on group 3
for PKG in \$("\${MYN[@]}" -e "SELECT id FROM server_packages WHERE enabled=1 AND id IN (1,2,3,4,9,10,11,12,5,6,7,8,13,14,15);"); do
  "\${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (${DE_PROSTO_GROUP},\$PKG);" 2>/dev/null || true
done
# Midrange packages on group 5
for PKG in \$("\${MYN[@]}" -e "SELECT id FROM server_packages WHERE enabled=1 AND id IN (17,18,19,20,21,10);"); do
  "\${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (${DE_MID_GROUP},\$PKG);" 2>/dev/null || true
done

NET_PROSTO=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=${DE_PROSTO_HV_ID} AND \`primary\`=1 LIMIT 1;")
NET_MID=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=\$HV_MID AND \`primary\`=1 LIMIT 1;")
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (${DE_BLOCK_ID},${DE_PROSTO_HV_ID});"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (${DE_BLOCK_ID},\$HV_MID);"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (${DE_BLOCK_ID},\$NET_PROSTO,${DE_PROSTO_HV_ID});"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (${DE_BLOCK_ID},\$NET_MID,\$HV_MID);"

cd /opt/virtfusion/app/control
for HV in ${DE_PROSTO_HV_ID} \$HV_MID; do
  echo "re-commission HV \$HV"
  printf "\${HV}\nyes\nyes\n" | \$PHP artisan hypervisor:re-commission 2>&1 | tail -6 || true
done

"\${MY[@]}" -e "SELECT id,name,ip,commissioned,hypervisor_group_id FROM hypervisors WHERE id IN (${DE_PROSTO_HV_ID},\$HV_MID);"
"\${MY[@]}" -e "SELECT ibh.hypervisor_id, h.name, ib.name FROM ip_block_hypervisor ibh JOIN ip_blocks ib ON ib.id=ibh.block_id JOIN hypervisors h ON h.id=ibh.hypervisor_id WHERE ib.id=${DE_BLOCK_ID};"
REMOTE

echo "=== 4. Portal: two DE nodes ==="
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"
export POSTGRES_DSN
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
-- Restore DE-prosto on 185.84.224.84
UPDATE vps.nodes
SET name = 'DE-1',
    vf_name = 'DE-prosto',
    vf_ip = '185.84.224.84',
    external_id = '3',
    supported_tiers = ARRAY['prosto', 'hustle']::text[],
    status = 'online',
    vf_enabled = true,
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003';

-- DE-midrange on 66.151.40.165 (new portal row)
INSERT INTO vps.nodes (id, name, region, external_id, status, capacity_instances, supported_tiers, vf_enabled, vf_commissioned, vf_ip, vf_name)
VALUES (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005',
  'DE-mid',
  'de',
  '5',
  'online',
  80,
  ARRAY['midrange']::text[],
  true,
  3,
  '66.151.40.165',
  'DE-midrange'
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  region = EXCLUDED.region,
  external_id = EXCLUDED.external_id,
  status = EXCLUDED.status,
  capacity_instances = EXCLUDED.capacity_instances,
  supported_tiers = EXCLUDED.supported_tiers,
  vf_enabled = EXCLUDED.vf_enabled,
  vf_commissioned = EXCLUDED.vf_commissioned,
  vf_ip = EXCLUDED.vf_ip,
  vf_name = EXCLUDED.vf_name,
  updated_at = now();

SELECT id, name, vf_name, vf_ip, external_id, supported_tiers FROM vps.nodes WHERE region = 'de' ORDER BY name;
SQL

echo "DE_DUAL_SETUP_DONE"
