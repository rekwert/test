#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_MID_SSH_PASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 "DE_MID_SSH_PASS='$DE_MID_SSH_PASS' bash -s" <<'NL'
echo "=== HV3 auth.json ==="
ssh -o BatchMode=yes root@185.84.224.84 'python3 -c "import json;d=json.load(open(\"/opt/virtfusion/app/hypervisor/conf/auth.json\"));print({k:(v if k!=\"token\" else v[:20]+\"...\"+str(len(v))) for k,v in d.items()})"'

echo "=== HV5 auth.json ==="
ssh -o BatchMode=yes root@66.151.40.165 'python3 -c "import json;d=json.load(open(\"/opt/virtfusion/app/hypervisor/conf/auth.json\"));print({k:(v if k!=\"token\" else v[:20]+\"...\"+str(len(v))) for k,v in d.items()})"'

echo "=== VF control test monitor HV3 ==="
cd /opt/virtfusion/app/control
/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
foreach ([3, 5] as $id) {
  try {
    $hv = App\Models\Hypervisor::find($id);
    echo "HV$id {$hv->name} ip={$hv->ip}\n";
    // try common connectivity method
    if (method_exists($hv, 'connectionTest')) {
      var_export($hv->connectionTest());
      echo "\n";
    }
  } catch (Throwable $e) {
    echo "HV$id err: " . $e->getMessage() . "\n";
  }
}
PHP

echo "=== Try artisan hypervisor:re-commission HV5 ==="
cd /opt/virtfusion/app/control
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "UPDATE hypervisors SET commissioned=0, token=NULL WHERE id=5;"
printf "5\nyes\nyes\n${DE_MID_SSH_PASS}\n" | timeout 120 /opt/virtfusion/php8/bin/php artisan hypervisor:re-commission 2>&1 | tail -30

mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
ssh -o BatchMode=yes root@66.151.40.165 'python3 -c "import json;d=json.load(open(\"/opt/virtfusion/app/hypervisor/conf/auth.json\"));print(len(d[\"token\"]))"' 2>/dev/null || echo no_auth

TOK=$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" 2>/dev/null || true)
if [[ -n "$TOK" ]]; then
  curl -sk -m 10 "https://66.151.40.165:8892/hypervisor/monitor" -H "Authorization: Bearer $TOK" -w "\nMON=%{http_code}\n" | tail -3
fi
supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL
