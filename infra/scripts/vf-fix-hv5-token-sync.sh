#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_PASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 "DE_PASS='${DE_PASS}' bash -s" <<'NL'
set -eo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
export SSHPASS="$DE_PASS"

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
$auth = ['ip' => '66.248.206.14', 'token' => substr($plain, 0, 200), 'hash' => $hash, 'id' => 5];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "plain_len=".strlen($plain)." db_comm=".$hv5->commissioned."\n";
PHP

sshpass -e scp -o StrictHostKeyChecking=no /tmp/hv5-auth.json root@66.151.40.165:/opt/virtfusion/app/hypervisor/conf/auth.json
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl start libvirtd vf-nginx; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

AUTH_TOK=$(sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" )
DB_PLAIN=$(/opt/virtfusion/php8/bin/php <<'PHP2'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
echo Crypt::decryptString(Hypervisor::find(5)->token);
PHP2
)

echo "prefix_match=$([ "${AUTH_TOK}" = "${DB_PLAIN:0:200}" ] && echo yes || echo no)"

curl -sk -m 15 "https://66.151.40.165:8892/hypervisor/monitor" -H "Authorization: Bearer ${AUTH_TOK}" -w "\nMON_authjson=%{http_code}\n" | tail -3
curl -sk -m 15 "https://66.151.40.165:8892/hypervisor/monitor" -H "Authorization: Bearer ${DB_PLAIN:0:200}" -w "\nMON_db200=%{http_code}\n" | tail -3

# HV3 baseline
DB3=$(/opt/virtfusion/php8/bin/php <<'PHP3'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
echo Crypt::decryptString(Hypervisor::find(3)->token);
PHP3
)
TOK3=$(ssh -o BatchMode=yes root@185.84.224.84 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" )
curl -sk -m 15 "https://185.84.224.84:8892/hypervisor/monitor" -H "Authorization: Bearer ${TOK3}" -w "\nHV3_authjson=%{http_code}\n" | tail -3
curl -sk -m 15 "https://185.84.224.84:8892/hypervisor/monitor" -H "Authorization: Bearer ${DB3:0:200}" -w "\nHV3_db200=%{http_code}\n" | tail -3

mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,commissioned,LENGTH(token) FROM hypervisors WHERE id IN (3,5);"
supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL
