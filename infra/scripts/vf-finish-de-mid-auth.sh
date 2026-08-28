#!/usr/bin/env bash
# DE-midrange ONLY: manual HV5 auth (artisan re-commission is a no-op for new HV).
# Does not touch DE-prosto host, FI, GB, or NL hypervisor agent.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
POSTGRES_DSN="${POSTGRES_DSN:?set POSTGRES_DSN}"
DE_MID_IP="66.151.40.165"

echo "=== 0. DE-prosto read-only ping ==="
ping -c1 -W2 185.84.224.84 >/dev/null && echo "DE-prosto OK" || echo "DE-prosto ping skip"

echo "=== 1. NL: generate HV5 token + auth.json (PHP Crypt) ==="
export SSHPASS="$NL_SSH_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
cd /opt/virtfusion/app/control

/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;

$ref = Hypervisor::find(3);
$hv5 = Hypervisor::find(5);
if (!$ref || !$hv5) { fwrite(STDERR, "HV3 or HV5 missing\n"); exit(1); }
$refPlain = Crypt::decryptString($ref->token);

function genPlain(int $len): string {
  $chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  $s = '';
  for ($i = 0; $i < $len; $i++) {
    $s .= $chars[random_int(0, strlen($chars) - 1)];
  }
  return $s;
}

$plain = genPlain(strlen($refPlain));
$hash = bin2hex(random_bytes(32));
$hv5->token = Crypt::encryptString($plain);
$hv5->commissioned = 3;
$hv5->enabled = 1;
$hv5->maintenance = 0;
$hv5->prohibit = 0;
$hv5->save();

$auth = [
  'ip' => '66.248.206.14',
  'token' => substr($plain, 0, 200),
  'hash' => $hash,
  'id' => 5,
];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "HV5 token db len=" . strlen((string)$hv5->token) . " plain len=" . strlen($plain) . "\n";
PHP

scp -o BatchMode=yes /tmp/hv5-auth.json root@66.151.40.165:/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@66.151.40.165 \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND state='failed' AND deleted_at IS NULL;"
"${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) tok,enabled FROM hypervisors WHERE id IN (3,5);"
ssh -o BatchMode=yes root@66.151.40.165 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo AUTH_OK || echo AUTH_MISSING'
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -3 || true
NL

echo "=== 2. DE-mid host: pool route + proxy_arp ==="
export SSHPASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"
sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_MID_IP" bash -s <<'MID'
set -euo pipefail
POOL_GW=212.102.227.1
POOL_CIDR=212.102.227.0/24
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1 >/dev/null
ip route show "$POOL_CIDR" 2>/dev/null | grep -q br0 || ip route add "$POOL_CIDR" dev br0 2>/dev/null || true
ip addr show dev br0 | grep -q "${POOL_GW}/" || ip addr add ${POOL_GW}/32 dev br0 2>/dev/null || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
grep -q '212.102.227.0/24' /etc/rc.local 2>/dev/null || {
  echo 'ip route add 212.102.227.0/24 dev br0 2>/dev/null || true' >> /etc/rc.local
  echo 'ip addr add 212.102.227.1/32 dev br0 2>/dev/null || true' >> /etc/rc.local
}
TOK=$(python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])")
curl -sk -o /dev/null -w "health=%{http_code}\n" -H "Authorization: Bearer $TOK" https://127.0.0.1:8892/health
curl -sk -o /dev/null -w "resources=%{http_code}\n" -H "Authorization: Bearer $TOK" https://127.0.0.1:8892/hypervisor/resources
MID

echo "=== 3. Portal DE-mid online ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
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
SELECT id, name, vf_name, vf_ip, external_id, supported_tiers, status, vf_commissioned FROM vps.nodes WHERE name ILIKE 'DE%';
SQL

echo "=== 4. VF API hypervisor 5 ==="
python3 - <<'PY'
import json, os, ssl, urllib.request
env = {}
for line in open("/opt/testVPStrade/infra/docker/.env"):
    if "=" in line and not line.strip().startswith("#"):
        k, v = line.strip().split("=", 1)
        env[k] = v.strip().strip('"').strip("'")
base = env["VIRTFUSION_API_URL"].rstrip("/")
key = env["VIRTFUSION_API_KEY"]
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
req = urllib.request.Request(
    base + "/compute/hypervisors/groups/5/resources",
    headers={"Authorization": "Bearer " + key, "Accept": "application/json"},
)
with urllib.request.urlopen(req, context=ctx, timeout=20) as r:
    d = json.load(r)["data"]
print("group5 accept=", d.get("accept"), "hypervisors=", len(d.get("hypervisors", [])))
PY

echo "DE_MID_AUTH_DONE"
