#!/usr/bin/env bash
# Complete DE-mid setup: auth repair, Midrange packages on HV group 5, capacity, retry order 225.
set -euo pipefail

ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
set +a

MID_IP=212.102.227.7
MID_PASS='xV1bFQjD7-'

echo "=== 1. Midrange packages in VirtFusion ==="
SSHPASS="$NL_SSH_PASS" sshpass -e scp \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no \
  "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" \
  root@66.248.206.14:/tmp/vf-fix-midrange-packages-all.sh
SSHPASS="$NL_SSH_PASS" sshpass -e ssh \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no \
  root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-fix-midrange-packages-all.sh && bash /tmp/vf-fix-midrange-packages-all.sh"

echo "=== 2. Link Midrange packages to DE-mid HV group 5 + network ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no \
  root@66.248.206.14 'bash -s' <<'REMOTE'
set -euo pipefail
set -a
source /opt/virtfusion/app/control/.env
set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

network_id=$("${MYN[@]}" -e \
  "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")

"${MY[@]}" -e \
  "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6, 5);
   INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id)
     VALUES (6, $network_id, 5);"

for package_id in 10 17 18 19 20 21; do
  "${MY[@]}" -e \
    "INSERT IGNORE INTO hypervisor_group_server_package
       (hypervisor_group_id, server_package_id)
     VALUES (5, $package_id);"
done

"${MY[@]}" -e \
  "UPDATE hypervisors
   SET ip='212.102.227.7', port=8892, ssh_port=22,
       commissioned=3, enabled=1, maintenance=0, prohibit=0,
       max_cpu=64, max_memory=384408, max_local_hdd=7000
   WHERE id=5;
   UPDATE hypervisor_storage SET capacity=7000, enabled=1
   WHERE hypervisor_id=5 AND path='/home/vf-data/disk';
   SELECT hypervisor_group_id, server_package_id
   FROM hypervisor_group_server_package WHERE hypervisor_group_id=5 ORDER BY server_package_id;
   SELECT id,name,cpu_cores,memory,storage,enabled FROM server_packages
   WHERE id IN (10,17,18,19,20,21) ORDER BY id;"
supervisorctl restart vf-queue-hv: vf-queue-control: >/dev/null 2>&1 || true
REMOTE

echo "=== 3. Repair DE-mid agent auth (copy working HV4 structure) ==="
SSHPASS="$GB_SSH_PASS" sshpass -e scp \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  root@212.108.83.47:/opt/virtfusion/app/hypervisor/conf/auth.json /tmp/hv4-auth.json

python3 - <<'PY'
import json
with open("/tmp/hv4-auth.json", encoding="utf-8") as fh:
    auth = json.load(fh)
auth["id"] = 5
with open("/tmp/hv5-auth-valid.json", "w", encoding="utf-8") as fh:
    json.dump(auth, fh, separators=(",", ":"))
PY

SSHPASS="$NL_SSH_PASS" sshpass -e ssh \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no \
  root@66.248.206.14 'bash -s' <<'REMOTE'
set -euo pipefail
set -a
source /opt/virtfusion/app/control/.env
set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "UPDATE hypervisors AS hv5
   JOIN hypervisors AS hv4 ON hv4.id = 4
   SET hv5.token = hv4.token
   WHERE hv5.id = 5;"
REMOTE

SSHPASS="$MID_PASS" sshpass -e scp \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  /tmp/hv5-auth-valid.json root@"$MID_IP":/opt/virtfusion/app/hypervisor/conf/auth.json

SSHPASS="$MID_PASS" sshpass -e ssh \
  -o ConnectTimeout=10 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  root@"$MID_IP" \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json;
   chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json;
   systemctl restart vf-php8-fpm vf-nginx;
   supervisorctl restart vf-queue-hv:'

echo "=== 4. Portal capacity + ensure plan map ==="
bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ROOT/infra/docker/.env"

psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes
SET max_memory_mb = 384408,
    max_disk_gb = 7000,
    status = 'online',
    maintenance_mode = false,
    vf_ip = '212.102.227.7',
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005'::uuid;
SQL

echo "=== 5. Retry order 225 if stuck ==="
psql "$POSTGRES_DSN" <<'SQL'
BEGIN;
UPDATE vps.instances
SET state = 'creating',
    external_id = NULL,
    ip_address = NULL,
    worker_poll_claimed_at = NULL,
    worker_poll_claimed_by = NULL,
    updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number = 225
) AND state IN ('queued', 'creating', 'error');

UPDATE vps.outbox
SET published = false,
    worker_poll_claimed_at = NULL,
    worker_poll_claimed_by = NULL
WHERE event_type = 'instance.provision_requested'
  AND payload->>'instance_id' IN (
    SELECT i.id::text FROM vps.instances i
    JOIN vps.orders o ON o.id = i.order_id
    WHERE o.order_number = 225
  );
COMMIT;

SELECT o.order_number, i.hostname, i.state, i.external_id, host(i.ip_address) AS ip
FROM vps.orders o
JOIN vps.instances i ON i.order_id = o.id
WHERE o.order_number = 225;
SQL

docker restart docker-vps-worker-1
echo "DE_MID_MIDRANGE_SETUP_DONE"
