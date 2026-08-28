#!/usr/bin/env bash
# Bootstrap DE (Frankfurt) prosto hypervisor on VirtFusion NL control panel.
# Run on NL panel host (66.248.206.14) as root.
# IP pool can be attached later when Hostkey delivers the block.
set -euo pipefail

DE_IP="${DE_IP:-185.84.224.84}"
DE_SSH_PASS="${DE_SSH_PASS:?set DE_SSH_PASS}"
DE_NAME="${DE_NAME:-DE-prosto}"
DE_GROUP_ID="${DE_GROUP_ID:-3}"
SKIP_IP_BLOCK="${SKIP_IP_BLOCK:-1}"

set -a
source /opt/virtfusion/app/control/.env
set +a

PHP="${PHP:-/opt/virtfusion/php8/bin/php}"
MYSQL=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)
MYSQLE=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

echo "=== Ensure NL control can SSH to DE (for commission) ==="
if [ ! -f /root/.ssh/id_ed25519 ]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS="$DE_SSH_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@"$DE_IP" \
  "mkdir -p /root/.ssh; grep -qxF '$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '$PUB' >> /root/.ssh/authorized_keys"
ssh -o StrictHostKeyChecking=no -o BatchMode=yes -o ConnectTimeout=15 root@"$DE_IP" 'hostname' || {
  echo "NL->DE SSH failed"; exit 1
}
echo "NL->DE SSH OK"

echo "=== Insert or update DE hypervisor row ==="
HV_ID=$("${MYSQL[@]}" -e "SELECT id FROM hypervisors WHERE ip='$DE_IP' LIMIT 1;" || true)
if [[ -z "${HV_ID:-}" ]]; then
  "${MYSQLE[@]}" -e "INSERT INTO hypervisors
    (type,arch,commissioned,name,ip,port,ssh_port,hypervisor_group_id,enabled,prohibit,nf_type,
     max_servers,max_cpu,max_memory,max_local_hdd,max_local_hdd_enabled,local_hdd_storage_type,
     maintenance,disk_type,disk_cache_type,backup_storage_type,default_cpu,default_machine_type,created_at,updated_at)
    VALUES (1,1,0,'$DE_NAME','$DE_IP',8892,22,1,1,0,4,
            50,64,65536,2000,1,0,0,'inherit','inherit',2,'inherit','inherit',NOW(),NOW());"
  HV_ID=$("${MYSQL[@]}" -e "SELECT id FROM hypervisors WHERE ip='$DE_IP' LIMIT 1;")
  echo "created hypervisor id=$HV_ID"
else
  "${MYSQLE[@]}" -e "UPDATE hypervisors SET
    name='$DE_NAME', enabled=1, maintenance=0, prohibit=0,
    max_servers=50, max_cpu=64, max_memory=65536, max_local_hdd=2000,
    ssh_port=22, port=8892
    WHERE id=$HV_ID;"
  echo "updated hypervisor id=$HV_ID"
fi

echo "=== DE hypervisor network (br0 bridge) ==="
NET_ID=$("${MYSQL[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=$HV_ID AND \`primary\`=1 LIMIT 1;" || true)
if [[ -z "${NET_ID:-}" ]]; then
  "${MYSQLE[@]}" -e "INSERT INTO hypervisor_networks
    (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
    VALUES ($HV_ID,'simpleBridge','br0',1,1,1,NOW(),NOW());"
  NET_ID=$("${MYSQL[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=$HV_ID AND \`primary\`=1 LIMIT 1;")
else
  "${MYSQLE[@]}" -e "UPDATE hypervisor_networks SET type='simpleBridge', bridge='br0', enabled=1 WHERE id=$NET_ID;"
fi
echo "network id=$NET_ID"

echo "=== DE storage on hypervisor ==="
STORAGE=$("${MYSQL[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=$HV_ID;")
if [[ "$STORAGE" == "0" ]]; then
  "${MYSQLE[@]}" -e "INSERT INTO hypervisor_storage
    (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES ($HV_ID,'Local disk','/home/vf-data/disk','mountpoint',2000,0,1,1,'[]',NOW(),NOW());"
fi
ssh -o StrictHostKeyChecking=no -o BatchMode=yes root@"$DE_IP" 'mkdir -p /home/vf-data/disk && chmod 755 /home/vf-data /home/vf-data/disk'

if [[ "$SKIP_IP_BLOCK" != "1" ]]; then
  echo "=== DE IP block (configure DE_POOL_* env vars to enable) ==="
else
  echo "=== Skipping IP block (Hostkey block pending) ==="
fi

echo "=== Create DE hypervisor group $DE_GROUP_ID ==="
GRP=$("${MYSQL[@]}" -e "SELECT COUNT(*) FROM hypervisor_groups WHERE id=$DE_GROUP_ID;")
if [[ "$GRP" == "0" ]]; then
  "${MYSQLE[@]}" -e "INSERT INTO hypervisor_groups
    (id,name,description,\`default\`,distribution_type,label,type,icon,visible,visible_label,enabled,created_at,updated_at)
    VALUES ($DE_GROUP_ID,'DE','Germany Frankfurt',0,5,NULL,NULL,NULL,0,1,1,NOW(),NOW());"
else
  "${MYSQLE[@]}" -e "UPDATE hypervisor_groups SET name='DE', enabled=1 WHERE id=$DE_GROUP_ID;"
fi
"${MYSQLE[@]}" -e "UPDATE hypervisors SET hypervisor_group_id=$DE_GROUP_ID WHERE id=$HV_ID;"

echo "=== Link packages to DE group ==="
for PKG in $("${MYSQL[@]}" -e "SELECT id FROM server_packages WHERE enabled=1;"); do
  "${MYSQLE[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
    VALUES ($DE_GROUP_ID,$PKG);" 2>/dev/null || true
done

echo "=== Trigger re-commission ==="
cd /opt/virtfusion/app/control
$PHP artisan hypervisor:re-commission --no-interaction 2>&1 | tail -25 || true

echo "=== Result ==="
"${MYSQLE[@]}" -e "SELECT id,name,ip,hypervisor_group_id,enabled,commissioned FROM hypervisors ORDER BY id;"
"${MYSQLE[@]}" -e "SELECT id,name FROM hypervisor_groups ORDER BY id;"
echo "DE_HV_ID=$HV_ID DE_GROUP_ID=$DE_GROUP_ID"
echo BOOTSTRAP_DE_DONE
