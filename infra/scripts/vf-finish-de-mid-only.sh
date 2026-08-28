#!/usr/bin/env bash
# Finish DE-midrange ONLY (66.151.40.165 / VF HV5). Does not touch DE-prosto, FI, NL, GB.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
[[ -f "$ROOT/infra/docker/.env.probe" ]] && source "$ROOT/infra/docker/.env.probe"
[[ -f "$ROOT/infra/docker/.env" ]] && source "$ROOT/infra/docker/.env"
DE_MID_PASS="${DE_MID_PASS:-${DE_MID_SSH_PASS:?DE_MID_PASS or DE_MID_SSH_PASS is required}}"
NL_SSH_PASS="${NL_SSH_PASS:?set NL_SSH_PASS in .env.probe}"
POSTGRES_DSN="${POSTGRES_DSN:?set POSTGRES_DSN}"
DE_MID_IP="66.151.40.165"

echo "=== 0. verify DE-prosto untouched (read-only) ==="
ping -c1 -W2 185.84.224.84 >/dev/null && echo "DE-prosto ping OK" || echo "DE-prosto ping fail (skip)"

echo "=== 1. DE-mid host prep ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_MID_IP" bash -s <<'MID'
set -euo pipefail
hostnamectl set-hostname DE-midrange
mkdir -p /home/vf-data/disk /home/vf-data/server
chmod 755 /home/vf-data /home/vf-data/disk

# Empty stub blocks installer — remove if agent not actually installed
if [[ -d /opt/virtfusion/app/hypervisor ]] && [[ ! -f /opt/virtfusion/app/hypervisor/update ]]; then
  rm -rf /opt/virtfusion/app/hypervisor
fi

if ! virsh uri >/dev/null 2>&1 || [[ ! -f /opt/virtfusion/app/hypervisor/update ]]; then
  echo "Installing VirtFusion hypervisor agent..."
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
  sleep 15
fi

# Route for shared DE pool (212.102.227.0/24) via br0 if not present
ip route show 212.102.227.0/24 2>/dev/null | grep -q br0 || ip route add 212.102.227.0/24 dev br0 2>/dev/null || true
ip addr show dev br0 | grep -q '212.102.227.1/' || ip addr add 212.102.227.1/32 dev br0 2>/dev/null || true
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1 >/dev/null 2>&1 || true
grep -q '212.102.227.0/24' /etc/rc.local 2>/dev/null || {
  grep -q 'rc.local' /etc/rc.local 2>/dev/null || echo '#!/bin/bash' > /etc/rc.local
  chmod +x /etc/rc.local
  grep -q '212.102.227.0/24' /etc/rc.local || echo 'ip route add 212.102.227.0/24 dev br0 2>/dev/null || true' >> /etc/rc.local
}

# Do not delete auth.json here — step 2 re-commission needs existing agent or clean install only
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2 || true
echo "host=$(hostname) kvm=$(test -e /dev/kvm && echo ok) agent=$(test -d /opt/virtfusion/app/hypervisor && echo ok)"
MID

echo "=== 2. NL: SSH key + HV5 auth token ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)
cd /opt/virtfusion/app/control

ssh-keygen -f /root/.ssh/known_hosts -R '${DE_MID_IP}' 2>/dev/null || true
ssh-keyscan -H '${DE_MID_IP}' >> /root/.ssh/known_hosts 2>/dev/null || true
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_MID_PASS}'
sshpass -e ssh -o StrictHostKeyChecking=no root@${DE_MID_IP} \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@${DE_MID_IP} hostname

"\${MY[@]}" -e "UPDATE hypervisors SET name='DE-midrange', ip='${DE_MID_IP}', hypervisor_group_id=5, enabled=1, maintenance=0, prohibit=0 WHERE id=5;"

ST=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=5;")
if [[ "\$ST" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_storage (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES (5,'Local disk','/home/vf-data/disk','mountpoint',7500,0,1,1,'[]',NOW(),NOW());"
fi
NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
if [[ -z "\${NET:-}" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_networks (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
    VALUES (5,'simpleBridge','br0',1,1,1,NOW(),NOW());"
  NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
fi
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,5);"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,\$NET,5);"

"\${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL AND state IN ('failed','complete');"

echo "--- HV5 auth (artisan re-commission is no-op for new HV; use PHP Crypt) ---"
\$PHP <<'PHPEOF'
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
$hv5->save();
$auth = ['ip' => '66.248.206.14', 'token' => substr($plain, 0, 200), 'hash' => bin2hex(random_bytes(32)), 'id' => 5];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "token_len=" . strlen((string)$hv5->token) . "\n";
PHPEOF
scp -o BatchMode=yes /tmp/hv5-auth.json root@${DE_MID_IP}:/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@${DE_MID_IP} 'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'
AUTH=\$(ssh -o BatchMode=yes root@${DE_MID_IP} 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO')
TOK=\$("\${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
COMM=\$("\${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
echo "commissioned=\$COMM token_len=\$TOK auth=\$AUTH"
[[ "\$COMM" == "3" && "\$TOK" -gt 500 && "\$AUTH" == "OK" ]] || exit 1
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -3 || true
"\${MY[@]}" -e "SELECT id,name,ip,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
REMOTE

echo "=== 3. Portal DE-mid online only ==="
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
SELECT id, name, vf_name, vf_ip, external_id, supported_tiers, status FROM vps.nodes WHERE name ILIKE 'DE%';
SQL

echo "=== 4. agent health ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_MID_IP" \
  'TOK=$(python3 -c "import json; print(json.load(open(\"/opt/virtfusion/app/hypervisor/conf/auth.json\"))[\"token\"])" 2>/dev/null); curl -sk -o /dev/null -w "health=%{http_code}\n" -H "Authorization: Bearer $TOK" https://127.0.0.1:8892/health'

echo "DE_MID_FINISH_DONE"
