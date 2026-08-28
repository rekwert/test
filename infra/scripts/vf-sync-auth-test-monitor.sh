#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_PASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"
SSHPASS="$NL_SSH_PASS" sshpass -e ssh root@66.248.206.14 "DE_PASS='$DE_PASS' bash -s" <<'NL'
set -eo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
export SSHPASS="$DE_PASS"

sync_auth() {
  local HV=$1 IP=$2
  /opt/virtfusion/php8/bin/php <<PHP
<?php
require __DIR__ . '/vendor/autoload.php';
\$app = require __DIR__ . '/bootstrap/app.php';
\$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
\$hv = Hypervisor::find($HV);
\$plain = Crypt::decryptString(\$hv->token);
\$auth = ['ip' => '66.248.206.14', 'token' => substr(\$plain, 0, 200), 'hash' => bin2hex(random_bytes(32)), 'id' => $HV];
file_put_contents("/tmp/hv${HV}-auth.json", json_encode(\$auth, JSON_UNESCAPED_SLASHES));
echo "HV$HV plain=".strlen(\$plain)." comm=".\$hv->commissioned."\n";
PHP
  sshpass -e scp -o StrictHostKeyChecking=no /tmp/hv${HV}-auth.json root@${IP}:/opt/virtfusion/app/hypervisor/conf/auth.json
  sshpass -e ssh -o StrictHostKeyChecking=no root@${IP} 'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json'
}

test_mon() {
  local HV=$1 IP=$2
  DB=$(/opt/virtfusion/php8/bin/php <<PHP
<?php
require __DIR__ . '/vendor/autoload.php';
\$app = require __DIR__ . '/bootstrap/app.php';
\$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
echo Crypt::decryptString(Hypervisor::find($HV)->token);
PHP
)
  AUTH=$(sshpass -e ssh -o StrictHostKeyChecking=no root@${IP} "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" )
  echo "=== HV$HV monitor tests ==="
  curl -sk -m 12 "https://${IP}:8892/hypervisor/monitor" -H "Authorization: Bearer ${AUTH}" -w " authjson=%{http_code}\n" -o /dev/null
  curl -sk -m 12 "https://${IP}:8892/hypervisor/monitor" -H "Authorization: Bearer ${DB:0:200}" -w " db200=%{http_code}\n" -o /dev/null
}

sync_auth 3 185.84.224.84
sync_auth 5 66.151.40.165
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 'systemctl start libvirtd vf-nginx; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -1'
test_mon 3 185.84.224.84
test_mon 5 66.151.40.165
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "UPDATE hypervisors SET commissioned=3 WHERE id IN (3,5);"
supervisorctl restart vf-queue: 2>/dev/null | tail -1
NL
