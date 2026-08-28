#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "DE_PASS='$DE_MID_SSH_PASS' bash -s" <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
PHP=/opt/virtfusion/php8/bin/php
DE_PASS="${DE_PASS:?missing DE_PASS}"

echo "=== Hypervisors DB ==="
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT id,name,ip,commissioned,enabled,LENGTH(token) encrypted_token_len FROM hypervisors WHERE id IN (3,5);"

echo "=== Decrypted DB token lengths ==="
$PHP <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
foreach ([3, 5] as $id) {
    $plain = Crypt::decryptString(Hypervisor::find($id)->token);
    echo "HV{$id} len=" . strlen($plain) . " sha256=" . hash('sha256', $plain) . PHP_EOL;
}
PHP

for SPEC in "3 185.84.224.84" "5 66.151.40.165"; do
  set -- $SPEC
  ID="$1"
  IP="$2"
  if [[ "$ID" == "5" ]]; then
    export SSHPASS="$DE_PASS"
    SSH=(sshpass -e ssh -o StrictHostKeyChecking=no)
  else
    SSH=(ssh -o BatchMode=yes -o StrictHostKeyChecking=no)
  fi
  echo "=== HV${ID} auth structure ==="
  "${SSH[@]}" root@"$IP" python3 - <<'PY'
import hashlib, json, os
p = "/opt/virtfusion/app/hypervisor/conf/auth.json"
d = json.load(open(p))
print({
    "owner": os.stat(p).st_uid,
    "mode": oct(os.stat(p).st_mode & 0o777),
    "keys": sorted(d),
    "types": {k: type(v).__name__ for k, v in d.items()},
    "lengths": {k: len(v) if isinstance(v, str) else None for k, v in d.items()},
    "id": d.get("id"),
    "ip": d.get("ip"),
    "token_sha256": hashlib.sha256(d.get("token", "").encode()).hexdigest(),
}
)
PY
  TOKEN=$("${SSH[@]}" root@"$IP" "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"")
  curl -sk -m 12 "https://${IP}:8892/hypervisor/monitor" \
    -H "Authorization: Bearer ${TOKEN}" -o "/tmp/hv${ID}-monitor" -w "HV${ID}_MONITOR=%{http_code}\n"
  tr '\n' ' ' < "/tmp/hv${ID}-monitor" | cut -c1-300
  echo
done

echo "=== Command signature/source ==="
$PHP artisan help hypervisor:re-commission | tr -d '\r'
rg -n -C 4 "hypervisor:re-commission" app routes 2>/dev/null || true

echo "=== HV5 agent auth errors ==="
export SSHPASS="$DE_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  "journalctl --since '20 minutes ago' -n 100 --no-pager | rg -i 'unauth|token|8892|hypervisor/monitor'" || true
NL
