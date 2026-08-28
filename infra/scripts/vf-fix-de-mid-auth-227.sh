#!/usr/bin/env bash
# Manual HV5 auth sync for DE-mid 212.102.227.7 (when artisan re-commission leaves commissioned=0).
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env
source /opt/testVPStrade/infra/docker/.env.probe
MID=212.102.227.7

echo "=== 1. Laravel token + auth.json on NL ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

$PHP <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
$ref = Hypervisor::find(3);
$hv5 = Hypervisor::find(5);
$refPlain = Crypt::decryptString($ref->token);
$chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
$plain = '';
for ($i = 0; $i < strlen($refPlain); $i++) {
  $plain .= $chars[random_int(0, strlen($chars) - 1)];
}
$hv5->token = Crypt::encryptString($plain);
$hv5->ip = '212.102.227.7';
$hv5->commissioned = 3;
$hv5->enabled = 1;
$hv5->maintenance = 0;
$hv5->prohibit = 0;
$hv5->save();
$auth = ['ip' => '66.248.206.14', 'token' => substr($plain, 0, 200), 'hash' => bin2hex(random_bytes(32)), 'id' => 5];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "plain_len=" . strlen($plain) . " auth_token_len=" . strlen($auth['token']) . " commissioned=" . $hv5->commissioned . "\n";
PHP

scp -o BatchMode=yes /tmp/hv5-auth.json root@212.102.227.7:/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -n -o BatchMode=yes root@212.102.227.7 \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl restart vf-php8-fpm vf-nginx; supervisorctl restart vf-queue-hv:' 2>&1 | tail -3

AUTHTOK=$(python3 -c 'import json; print(json.load(open("/tmp/hv5-auth.json"))["token"])')
curl -sk -m15 "https://212.102.227.7:8892/hypervisor/monitor" -H "Authorization: Bearer $AUTHTOK" -w "\nmonitor_http=%{http_code}\n" | tail -5
curl -sk -m15 "https://212.102.227.7:8892/health" -H "Authorization: Bearer $AUTHTOK" -w "\nhealth_http=%{http_code}\n"

"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) tok FROM hypervisors WHERE id=5;"
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -3 || true
NL

echo "=== 2. Portal DE-mid + cleanup failed VF servers ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes SET status='online', vf_commissioned=3, vf_enabled=true, vf_ip='212.102.227.7', maintenance_mode=false, updated_at=now()
WHERE name='DE-mid';
SQL

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "UPDATE ipv4 SET server_id=NULL, interface_id=NULL WHERE server_id IN (693,699,700);
   DELETE FROM servers WHERE id IN (693,699,700);"
NL

echo "=== 3. Retry order 225 ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.instances SET state='creating', external_id=NULL, ip_address=NULL,
  worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id IN (SELECT i.id FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number=225);
UPDATE vps.outbox SET published=false, worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL
WHERE event_type='instance.provision_requested'
  AND payload->>'instance_id' IN (SELECT i.id::text FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id WHERE o.order_number=225);
SQL

cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml up -d --force-recreate vps-worker 2>/dev/null || docker restart docker-vps-worker-1
echo "wait 180s..."
sleep 180

psql "$POSTGRES_DSN" -c "
SELECT o.order_number,i.state,i.external_id,host(i.ip_address) ip,i.hostname,n.name
FROM vps.orders o JOIN vps.instances i ON i.order_id=o.id LEFT JOIN vps.nodes n ON n.id=i.node_id
WHERE o.order_number=225;"

docker logs docker-vps-worker-1 --since 4m 2>&1 | grep -iE '225|273f444b|complete provision|provision failed|connectivity|not commissioned' | tail -15 || true

SSHPASS='xV1bFQjD7-' sshpass -e ssh -o StrictHostKeyChecking=no root@212.102.227.7 'virsh list --all' 2>&1

echo DE_MID_AUTH_AND_RETRY_DONE
