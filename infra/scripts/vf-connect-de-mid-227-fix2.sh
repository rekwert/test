#!/usr/bin/env bash
# Finish DE-mid connect: plan map, agent auth, SS package groups, cleanup 693, retry 225.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
set +a
MID_IP=212.102.227.7
MID_PASS='xV1bFQjD7-'

echo "=== 1. Fix plan map + restart worker ==="
bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ROOT/infra/docker/.env"
grep VIRTFUSION_PLAN_MAP "$ROOT/infra/docker/.env" | tr ',' '\n' | grep 111111111711
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml up -d vps-worker 2>/dev/null || docker restart docker-vps-worker-1
sleep 5
docker exec docker-vps-worker-1 printenv VIRTFUSION_PLAN_MAP | tr ',' '\n' | grep 111111111711

echo "=== 2. VF: SS groups + delete failed 693 + auth ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "MID_IP='$MID_IP' MID_PASS='$MID_PASS' bash -s" <<'REMOTE'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
cd /opt/virtfusion/app/control

ssh-keygen -f /root/.ssh/known_hosts -R "$MID_IP" 2>/dev/null || true
ssh-keyscan -H "$MID_IP" >> /root/.ssh/known_hosts 2>/dev/null || true
export SSHPASS="$MID_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID_IP" hostname

EXISTS=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")
if [[ "$EXISTS" == "0" ]]; then
  NEXT=$("${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES ($NEXT, 5, 'DE Mid', 'DE Mid', 5);"
fi
GID=$("${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
EXISTS2=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;")
if [[ "$EXISTS2" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 5, $GID, 5);"
fi
"${MY[@]}" -e "SELECT * FROM ss_group_hv_group WHERE hypervisor_group_id=5;
SELECT * FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;"

"${MY[@]}" -e "UPDATE ipv4 SET server_id=NULL, interface_id=NULL WHERE server_id=693;
UPDATE servers SET deleted_at=NOW(), state='deleted' WHERE id=693;"

$PHP <<'PHPEOF'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
$ref = Hypervisor::find(3);
$hv5 = Hypervisor::find(5);
$plain = '';
$chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
$len = strlen(Crypt::decryptString($ref->token));
for ($i = 0; $i < $len; $i++) $plain .= $chars[random_int(0, strlen($chars) - 1)];
$hv5->token = Crypt::encryptString($plain);
$hv5->commissioned = 3;
$hv5->enabled = 1;
$hv5->maintenance = 0;
$hv5->prohibit = 0;
$hv5->ip = '212.102.227.7';
$hv5->save();
$auth = ['ip' => '66.248.206.14', 'token' => substr($plain, 0, 200), 'hash' => bin2hex(random_bytes(32)), 'id' => 5];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "token_ok len=" . strlen((string)$hv5->token) . "\n";
PHPEOF

scp -o BatchMode=yes /tmp/hv5-auth.json root@"$MID_IP":/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@"$MID_IP" \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl restart vf-php8-fpm vf-nginx; supervisorctl restart vf-queue-hv:'
TOK=$(python3 -c 'import json; print(json.load(open("/tmp/hv5-auth.json"))["token"])')
curl -sk -m10 "https://$MID_IP:8892/health" -H "Authorization: Bearer $TOK" -w "\nagent_http=%{http_code}\n"
supervisorctl restart vf-queue: vf-queue-hv: vf-queue-control: 2>/dev/null | tail -3 || true
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) tok FROM hypervisors WHERE id=5;"
REMOTE

echo "=== 3. Retry order 225 ==="
psql "$POSTGRES_DSN" <<'SQL'
BEGIN;
UPDATE vps.instances
SET state='creating', external_id=NULL, ip_address=NULL,
    worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id IN (SELECT i.id FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number=225);
UPDATE vps.outbox SET published=false, worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL
WHERE event_type='instance.provision_requested'
  AND payload->>'instance_id' IN (SELECT i.id::text FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number=225);
COMMIT;
SQL
docker restart docker-vps-worker-1
echo "wait 150s..."
sleep 150

psql "$POSTGRES_DSN" -c "
SELECT o.order_number,i.hostname,i.state,i.external_id,host(i.ip_address) ip,n.name
FROM vps.orders o JOIN vps.instances i ON i.order_id=o.id LEFT JOIN vps.nodes n ON n.id=i.node_id
WHERE o.order_number=225;"
docker logs docker-vps-worker-1 --since 4m 2>&1 | grep -iE '225|273f444b|complete provision|provision failed|Invalid package|1777' | tail -15 || true

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT s.id,s.state,s.hypervisor_id,INET_NTOA(v4.address) ip FROM servers s LEFT JOIN ipv4 v4 ON v4.server_id=s.id WHERE s.hypervisor_id=5 AND s.deleted_at IS NULL ORDER BY s.id DESC LIMIT 3;"
NL
echo DE_MID_FIX2_DONE
