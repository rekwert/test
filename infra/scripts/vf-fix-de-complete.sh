#!/usr/bin/env bash
# Complete DE-midrange fix: copy HV3 commission artifacts, restore auth, rebuild orders.
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' /opt/testVPStrade/infra/docker/.env | cut -d= -f2-)"

echo "=== 1. NL: copy HV3 settings/config to HV5 + proper token/auth ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

# Copy hypervisor_settings HV3 -> HV5
"${MY[@]}" -e "DELETE FROM hypervisor_settings WHERE hypervisor_id=5;"
"${MY[@]}" -e "INSERT INTO hypervisor_settings (hypervisor_id, force_ipv6, timezone, vnc_listen_type, display_name, cpu_set, disk_driver_io, created_at, updated_at)
  SELECT 5, force_ipv6, timezone, vnc_listen_type, display_name, cpu_set, disk_driver_io, NOW(), NOW() FROM hypervisor_settings WHERE hypervisor_id=3;"

# Copy hypervisor_config HV3 -> HV5
"${MY[@]}" -e "DELETE FROM hypervisor_config WHERE hypervisor_id=5;"
"${MY[@]}" -e "INSERT INTO hypervisor_config (hypervisor_id, \`key\`, value, created_at, updated_at)
  SELECT 5, \`key\`, value, NOW(), NOW() FROM hypervisor_config WHERE hypervisor_id=3;"

echo "settings/config copied"

cd /opt/virtfusion/app/control
# Generate token matching HV3 format + auth.json
/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;

$ref = Hypervisor::find(3);
$hv5 = Hypervisor::find(5);
$refPlain = Crypt::decryptString($ref->token);
$refAuth = json_decode(trim(shell_exec('ssh -o BatchMode=yes root@185.84.224.84 cat /opt/virtfusion/app/hypervisor/conf/auth.json')), true);

function genPlain($len) {
  $chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  $s = '';
  for ($i=0; $i<$len; $i++) $s .= $chars[random_int(0, strlen($chars)-1)];
  return $s;
}

$plain = genPlain(strlen($refPlain));
$hash = bin2hex(random_bytes(32));
$hv5->token = Crypt::encryptString($plain);
$hv5->commissioned = 3;
$hv5->enabled = 1;
$hv5->save();

$auth = [
  'ip' => '66.248.206.14',
  'token' => substr($plain, 0, 200),
  'hash' => $hash,
  'id' => 5,
];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "token db len=".strlen($hv5->token)." plain len=".strlen($plain)."\n";
PHP

scp -o BatchMode=yes /tmp/hv5-auth.json root@66.151.40.165:/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@66.151.40.165 'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

# Cleanup failed VF servers
for SID in 645 647 649 651 652; do
  "${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE id=$SID AND deleted_at IS NULL;" 2>/dev/null || true
done

"${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) FROM hypervisors WHERE id IN (3,5);"
"${MY[@]}" -e "SELECT COUNT(*) cfg FROM hypervisor_config WHERE hypervisor_id=5;"
ssh -o BatchMode=yes root@66.151.40.165 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo AUTH_OK || echo AUTH_MISSING'

supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -5 || true
NL

echo "=== 2. Portal: reset mid orders ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET external_id = NULL, ip_address = NULL, state = 'creating', updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number IN (199, 196) AND i.state IN ('creating', 'error')
);
UPDATE vps.nodes SET vf_commissioned = 3, status = 'online', vf_enabled = true, updated_at = now()
WHERE id IN ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005');
SQL

echo "=== 3. Restart worker ==="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "=== 4. Wait 150s for builds ==="
sleep 150

echo "=== 5. Results ==="
docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE 'complete provision|retry allocate|enough resources|connectivity|not commissioned|651|652|653|654|199|196|198' | tail -30 || true

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "mysql -h127.0.0.1 -u\$(grep DB_USERNAME /opt/virtfusion/app/control/.env|cut -d= -f2) -p\$(grep DB_PASSWORD /opt/virtfusion/app/control/.env|cut -d= -f2) \$(grep DB_DATABASE /opt/virtfusion/app/control/.env|cut -d= -f2) -e 'SELECT id,hypervisor_id,state,commissioned FROM servers WHERE hypervisor_id=5 ORDER BY id DESC LIMIT 5;'"

SSHPASS="$DE_MID_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 'virsh list --all | head -8'

psql "$POSTGRES_DSN" -c \
  "SELECT o.order_number, i.hostname, i.state, i.ip_address, i.external_id, n.name
   FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id
   LEFT JOIN vps.nodes n ON n.id=i.node_id
   WHERE o.order_number IN (198,199,196);"

echo "DE_COMPLETE_FIX_DONE"
