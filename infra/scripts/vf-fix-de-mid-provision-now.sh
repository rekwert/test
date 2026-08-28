#!/usr/bin/env bash
set -euo pipefail

ROOT="${ROOT:-/opt/testVPStrade}"
MID_IP=212.102.227.7
MID_PASS='xV1bFQjD7-'

set -a
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
set +a

echo "=== 1. Fix corrupted Midrange-1 plan map (17777 -> 17) ==="
bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ROOT/infra/docker/.env"
grep '11111111-1111-1111-1111-111111111711' "$ROOT/infra/docker/.env"

echo "=== 2. Midrange packages + HV5 links in VirtFusion ==="
SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" root@66.248.206.14:/tmp/
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-fix-midrange-packages-all.sh && bash /tmp/vf-fix-midrange-packages-all.sh"

echo "=== 3. NL SSH key to DE-mid + network pool + capacity ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 'bash -s' <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(mysql -N -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")

if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${MID_PASS}'
ssh-keygen -f /root/.ssh/known_hosts -R '${MID_IP}' >/dev/null 2>&1 || true
sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@${MID_IP} \
  "mkdir -p /root/.ssh; grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
unset SSHPASS
ssh -o BatchMode=yes -o ConnectTimeout=10 root@${MID_IP} hostname

network_id=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
"\${MY[@]}" -e \
  "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6, 5);
   INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6, \$network_id, 5);
   UPDATE hypervisors SET ip='${MID_IP}', commissioned=3, enabled=1, maintenance=0, prohibit=0,
     max_cpu=64, max_memory=384408, max_local_hdd=7000 WHERE id=5;
   UPDATE hypervisor_storage SET capacity=7000 WHERE hypervisor_id=5;"

cd /opt/virtfusion/app/control
printf '5\nyes\nyes\n${MID_PASS}\n' | /opt/virtfusion/php8/bin/php artisan hypervisor:re-commission 2>&1 | tail -8
"\${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,maintenance FROM hypervisors WHERE id=5;"
supervisorctl restart vf-queue-hv: vf-queue-control: >/dev/null 2>&1 || true
REMOTE

echo "=== 4. Portal: capacity + retry order 225 ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes
SET max_memory_mb=384408, max_disk_gb=7000, status='online', maintenance_mode=false,
    vf_ip='212.102.227.7', updated_at=now()
WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005'::uuid;

BEGIN;
UPDATE vps.instances
SET state='creating', external_id=NULL, ip_address=NULL,
    worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id IN (
  SELECT i.id FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id
  WHERE o.order_number=225
) AND state IN ('queued','creating','error');

UPDATE vps.outbox
SET published=false, worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL
WHERE event_type='instance.provision_requested'
  AND payload->>'instance_id' IN (
    SELECT i.id::text FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id
    WHERE o.order_number=225
  );
COMMIT;

SELECT o.order_number,i.hostname,i.state,i.external_id,host(i.ip_address) ip
FROM vps.orders o JOIN vps.instances i ON i.order_id=o.id WHERE o.order_number=225;
SQL

docker restart docker-vps-worker-1
echo "Waiting 90s for provision..."
sleep 90
docker logs docker-vps-worker-1 --since 2m 2>&1 | python3 -c 'import sys
for line in sys.stdin:
    if any(x in line.lower() for x in ("273f444b","225","midrange","invalid package","complete provision","provision failed","212.102")):
        print(line, end="")'
psql "$POSTGRES_DSN" -c "SELECT o.order_number,i.hostname,i.state,i.external_id,host(i.ip_address) ip FROM vps.orders o JOIN vps.instances i ON i.order_id=o.id WHERE o.order_number=225;"
echo "DE_MID_FIX_DONE"
