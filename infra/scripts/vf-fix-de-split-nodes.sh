#!/usr/bin/env bash
# Fix DE routing: prosto->HV3 (185.84.224.84), midrange->HV5 (66.151.40.165)
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"
DE_MID_PASS="${DE_MID_SSH_PASS:?set DE_MID_SSH_PASS}"

echo "=== STEP 0: current state ==="
psql "$POSTGRES_DSN" -c "SELECT id,name,vf_name,vf_ip,external_id,supported_tiers FROM vps.nodes WHERE region='de';"

echo "=== STEP 1: DE-mid host prep ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'MID'
set -euo pipefail
hostnamectl set-hostname DE-midrange || true
mkdir -p /home/vf-data/disk /opt/virtfusion/app/hypervisor/conf
chmod 755 /home/vf-data /home/vf-data/disk
if ! virsh uri >/dev/null 2>&1; then
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
  sleep 12
fi
rm -f /opt/virtfusion/app/hypervisor/conf/auth.json
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2 || true
echo "host=$(hostname) kvm=$(test -e /dev/kvm && echo ok || echo no) agent=$(test -d /opt/virtfusion/app/hypervisor && echo ok || echo no)"
MID

echo "=== STEP 2: NL commission HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)
cd /opt/virtfusion/app/control

# SSH key
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_MID_PASS}'
ssh-keygen -f /root/.ssh/known_hosts -R '66.151.40.165' 2>/dev/null || true
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@66.151.40.165 hostname

# Ensure HV5 row sane
"\${MY[@]}" -e "UPDATE hypervisors SET name='DE-midrange', ip='66.151.40.165', hypervisor_group_id=5, enabled=1, maintenance=0, prohibit=0 WHERE id=5;"

ST=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=5;")
if [[ "\$ST" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_storage (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES (5,'Local disk','/home/vf-data/disk','mountpoint',8000,0,1,1,'[]',NOW(),NOW());"
fi
NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
if [[ -z "\${NET:-}" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_networks (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
    VALUES (5,'simpleBridge','br0',1,1,1,NOW(),NOW());"
  NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
fi
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,5);"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,\$NET,5);"

# SS group 5
EXISTS=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")
if [[ "\$EXISTS" == "0" ]]; then
  NEXT=\$("\${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "\${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES (\$NEXT, 5, 'DE Mid', 'DE Mid', 5);"
fi
GID=\$("\${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
"\${MY[@]}" -e "INSERT IGNORE INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 5, \$GID, 5);"
for PKG in 10 17 18 19 20 21; do
  "\${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (5,\$PKG);"
done

# Soft-delete zombies on HV5
"\${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL;"

echo "--- re-commission HV5 ---"
"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, token=NULL WHERE id=5;"
printf "5\nyes\nyes\n${DE_MID_PASS}\n" | script -q -c "\$PHP artisan hypervisor:re-commission" /dev/null 2>&1 | tee /tmp/hv5-split-comm.log | tail -20

COMM=0; TOK=0
for i in \$(seq 1 36); do
  sleep 10
  COMM=\$("\${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
  TOK=\$("\${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
  AUTH=\$(ssh -o BatchMode=yes root@66.151.40.165 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO' 2>/dev/null || echo SSHFAIL)
  echo "poll \$i commissioned=\$COMM token_len=\$TOK auth=\$AUTH"
  [[ "\$COMM" == "3" && "\$TOK" -gt 500 && "\$AUTH" == "OK" ]] && break
done

if [[ "\$COMM" != "3" || "\$TOK" -lt 500 ]]; then
  echo "HV5 COMMISSION FAILED"
  tail -30 /tmp/hv5-split-comm.log
  exit 1
fi

"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1 WHERE id IN (3,5);"
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -3 || true
ssh -o BatchMode=yes root@66.151.40.165 'supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2' || true
"\${MY[@]}" -e "SELECT id,name,ip,commissioned,LENGTH(token) tok FROM hypervisors WHERE id IN (3,5);"
REMOTE

echo "=== STEP 3: Portal split nodes ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
-- Allow distinct external_id per node
DROP INDEX IF EXISTS vps.nodes_external_id_uidx;

UPDATE vps.nodes SET
  external_id = '3',
  vf_name = 'DE-prosto',
  vf_ip = '185.84.224.84',
  status = 'online',
  vf_commissioned = 3,
  vf_enabled = true,
  maintenance_mode = false,
  updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003';

UPDATE vps.nodes SET
  external_id = '5',
  vf_name = 'DE-midrange',
  vf_ip = '66.151.40.165',
  status = 'online',
  vf_commissioned = 3,
  vf_enabled = true,
  maintenance_mode = false,
  updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

SELECT id, name, vf_name, vf_ip, external_id, supported_tiers, status FROM vps.nodes WHERE region='de' ORDER BY name;
SQL

echo "=== STEP 4: restart worker ==="
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "DE_SPLIT_NODES_DONE"
