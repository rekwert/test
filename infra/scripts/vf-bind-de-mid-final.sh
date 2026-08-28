#!/usr/bin/env bash
# Bind DE-mid 212.102.227.7: copy working HV4 auth+token, cleanup, retry order 225.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
set +a

MID_IP=212.102.227.7
MID_PASS='xV1bFQjD7-'

echo "=== 1. Copy HV4 auth.json -> HV5 on DE-mid ==="
SSHPASS="$GB_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  root@212.108.83.47:/opt/virtfusion/app/hypervisor/conf/auth.json /tmp/hv4-auth.json

python3 - <<'PY'
import json
with open("/tmp/hv4-auth.json", encoding="utf-8") as fh:
    auth = json.load(fh)
auth["id"] = 5
with open("/tmp/hv5-auth-valid.json", "w", encoding="utf-8") as fh:
    json.dump(auth, fh, separators=(",", ":"))
print("auth id=5 token_len", len(auth.get("token", "")))
PY

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "
  UPDATE hypervisors AS hv5
  JOIN hypervisors AS hv4 ON hv4.id = 4
  SET hv5.token = hv4.token,
      hv5.commissioned = 3,
      hv5.enabled = 1,
      hv5.maintenance = 0,
      hv5.prohibit = 0,
      hv5.ip = '212.102.227.7',
      hv5.port = 8892
  WHERE hv5.id = 5;"
"${MY[@]}" -e "SELECT id,name,ip,commissioned,LENGTH(token) tok FROM hypervisors WHERE id IN (4,5);"
supervisorctl restart vf-queue: vf-queue-hv: vf-queue-control: 2>/dev/null | tail -3 || true
NL

SSHPASS="$MID_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  /tmp/hv5-auth-valid.json root@"$MID_IP":/opt/virtfusion/app/hypervisor/conf/auth.json
SSHPASS="$MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID_IP" \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json;
   chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json;
   systemctl restart vf-php8-fpm vf-nginx;
   supervisorctl restart vf-queue-hv:'

echo "=== 2. SS groups + midrange packages (if missing) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" root@66.248.206.14:/tmp/
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-fix-midrange-packages-all.sh && bash /tmp/vf-fix-midrange-packages-all.sh" 2>&1 | tail -5

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
NET=$("${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
"${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,5);
  INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,$NET,5);"
if [ "$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")" = "0" ]; then
  GID=$("${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES ($GID, 5, 'DE-mid', 'DE Midrange', 5);"
fi
if [ "$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;")" = "0" ]; then
  SS=$("${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
  "${MY[@]}" -e "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 5, $SS, 5);"
fi
"${MY[@]}" -e "SELECT * FROM ss_group_hv_group WHERE hypervisor_group_id=5;
  SELECT * FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;"
NL

echo "=== 3. Pool routing on DE-mid ==="
SSHPASS="$MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID_IP" bash -s <<'MID'
ip route replace 212.102.227.0/24 dev br0 scope link 2>/dev/null || true
ip addr show dev br0 | grep -q '212.102.227.1/' || ip addr add 212.102.227.1/32 dev br0 2>/dev/null || true
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1 >/dev/null
echo "br0=$(ip -br addr show br0 | head -1)"
MID

echo "=== 4. Cleanup failed VF servers on HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "
  UPDATE ipv4 SET server_id=NULL, interface_id=NULL
  WHERE server_id IN (SELECT id FROM (SELECT id FROM servers WHERE hypervisor_id=5 AND state IN ('failed','allocated')) t);
  DELETE FROM servers WHERE hypervisor_id=5 AND state IN ('failed','allocated');"
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT id,state FROM servers WHERE hypervisor_id=5 ORDER BY id DESC LIMIT 5;"
NL

bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ROOT/infra/docker/.env" 2>&1 | tail -3

echo "=== 5. Portal DE-mid online ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes SET
  external_id='5', vf_name='DE-midrange', vf_ip='212.102.227.7',
  status='online', vf_commissioned=3, vf_enabled=true, maintenance_mode=false,
  max_memory_mb=384408, max_disk_gb=7000, updated_at=now()
WHERE name='DE-mid';
SQL

echo "=== 6. Retry order 225 ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.instances SET state='creating', external_id=NULL, ip_address=NULL,
  worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id IN (SELECT i.id FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number=225);
UPDATE vps.outbox SET published=false, worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL
WHERE event_type='instance.provision_requested'
  AND payload->>'instance_id' IN (SELECT i.id::text FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number=225);
SQL

cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml up -d --force-recreate vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "=== 7. Wait 150s ==="
sleep 150

psql "$POSTGRES_DSN" -c "
SELECT o.order_number,i.state,i.external_id,host(i.ip_address) ip,i.hostname,n.name
FROM vps.orders o JOIN vps.instances i ON i.order_id=o.id LEFT JOIN vps.nodes n ON n.id=i.node_id
WHERE o.order_number=225;"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT id,state,hypervisor_id FROM servers WHERE hypervisor_id=5 ORDER BY id DESC LIMIT 3;"
NL

docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE '225|273f444b|complete provision|provision failed|connectivity|license|Invalid package' | tail -20 || true

SSHPASS="$MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID_IP" 'virsh list --all' 2>&1

echo DE_MID_BOUND_OK
