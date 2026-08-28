#!/usr/bin/env bash
# Bind DE-mid 212.102.227.7 (HV5) — official re-commission + cleanup + retry order 225.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
set +a

MID=212.102.227.7
MID_PASS='xV1bFQjD7-'

echo "=== 1. NL: SSH key + official re-commission HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
cd /opt/virtfusion/app/control
MID=212.102.227.7
MID_PASS='xV1bFQjD7-'

ssh-keygen -f /root/.ssh/known_hosts -R "$MID" 2>/dev/null || true
ssh-keyscan -H "$MID" >> /root/.ssh/known_hosts 2>/dev/null || true
[ -f /root/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
PUB=$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS="$MID_PASS"
sshpass -e ssh -n -o StrictHostKeyChecking=no root@"$MID" \
  "grep -qxF '$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '$PUB' >> /root/.ssh/authorized_keys"
unset SSHPASS
ssh -n -o BatchMode=yes root@"$MID" hostname

NET=$("${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
"${MY[@]}" -e "
  UPDATE hypervisors SET name='DE-midrange', ip='$MID', port=8892, ssh_port=22,
    hypervisor_group_id=5, enabled=1, maintenance=0, prohibit=0 WHERE id=5;
  INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,5);
  INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,$NET,5);"

# Wipe manual auth — only official re-commission creates valid auth.json hash
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, token=NULL WHERE id=5;"
ssh -n -o BatchMode=yes root@"$MID" 'rm -f /opt/virtfusion/app/hypervisor/conf/auth.json'

echo "=== re-commission (timeout 180s) ==="
printf "5\nyes\nyes\n$MID_PASS\n" | timeout 180 $PHP artisan hypervisor:re-commission 2>&1 | tee /tmp/hv5-recomm-final.log | tail -35

COMM=$("${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
TOK=$("${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
AUTH=$(ssh -n -o BatchMode=yes root@"$MID" 'test -s /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO' 2>/dev/null || echo SSHFAIL)
echo "post_recomm commissioned=$COMM token_len=$TOK auth=$AUTH"

# If re-commission didn't finish, poll up to 2 min
if [[ "$AUTH" != "OK" || "$COMM" != "3" ]]; then
  for i in $(seq 1 12); do
    sleep 10
    COMM=$("${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
    TOK=$("${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
    AUTH=$(ssh -n -o BatchMode=yes root@"$MID" 'test -s /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO' 2>/dev/null || echo SSHFAIL)
    echo "poll $i commissioned=$COMM token_len=$TOK auth=$AUTH"
    [[ "$COMM" == "3" && "$TOK" -gt 500 && "$AUTH" == "OK" ]] && break
  done
fi

if [[ "$AUTH" != "OK" ]]; then
  echo "=== re-commission FAILED — log tail ==="
  tail -80 /tmp/hv5-recomm-final.log
  exit 1
fi

ssh -n -o BatchMode=yes root@"$MID" \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl restart vf-php8-fpm vf-nginx; supervisorctl restart vf-queue-hv:' 2>&1 | tail -3

echo "=== connectivity probe (panel-style paths) ==="
TOK=$(ssh -n -o BatchMode=yes root@"$MID" 'python3 -c "import json;print(json.load(open(\"/opt/virtfusion/app/hypervisor/conf/auth.json\"))[\"token\"])"')
for path in /local/hypervisor/monitor /local/hypervisor/resources /hypervisor/monitor /health; do
  code=$(curl -sk -m12 -o /tmp/agent_out -w '%{http_code}' -H "Authorization: Bearer $TOK" "https://$MID:8892$path")
  body=$(head -c 120 /tmp/agent_out)
  echo "$path -> $code $body"
done

# Compare HV3 working agent path
TOK3=$(ssh -n -o BatchMode=yes root@212.102.227.6 'python3 -c "import json;print(json.load(open(\"/opt/virtfusion/app/hypervisor/conf/auth.json\"))[\"token\"])"' 2>/dev/null || true)
if [ -n "$TOK3" ]; then
  code=$(curl -sk -m12 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK3" "https://212.102.227.6:8892/local/hypervisor/monitor")
  echo "HV3 /local/hypervisor/monitor -> $code"
fi

"${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1 WHERE id=5;"
"${MY[@]}" -e "SELECT id,name,ip,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
supervisorctl restart vf-queue: vf-queue-hv: vf-queue-control: 2>/dev/null | tail -3 || true
NL

echo "=== 2. Portal DE-mid + SS groups ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes SET
  external_id='5', vf_name='DE-midrange', vf_ip='212.102.227.7',
  status='online', vf_commissioned=3, vf_enabled=true, maintenance_mode=false,
  updated_at=now()
WHERE name='DE-mid';
SQL

SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" root@66.248.206.14:/tmp/
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-fix-midrange-packages-all.sh && bash /tmp/vf-fix-midrange-packages-all.sh" 2>&1 | tail -8

echo "=== 3. Cleanup failed VF servers on HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "
  UPDATE ipv4 SET server_id=NULL, interface_id=NULL WHERE server_id IN (SELECT id FROM (SELECT id FROM servers WHERE hypervisor_id=5 AND state='failed') t);
  DELETE FROM servers WHERE hypervisor_id=5 AND state='failed';"
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT id,state FROM servers WHERE hypervisor_id=5 ORDER BY id DESC LIMIT 5;"
NL

echo "=== 4. Retry order 225 ==="
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

echo "=== 5. Wait 120s and check ==="
sleep 120

psql "$POSTGRES_DSN" -c "
SELECT o.order_number,i.state,i.external_id,host(i.ip_address) ip,i.hostname,n.name
FROM vps.orders o JOIN vps.instances i ON i.order_id=o.id LEFT JOIN vps.nodes n ON n.id=i.node_id
WHERE o.order_number=225;"

docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE '225|273f444b|complete provision|provision failed|connectivity|retry build|Invalid package' | tail -20 || true

SSHPASS="$MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID" 'virsh list --all' 2>&1

echo DE_MID_BIND_DONE
