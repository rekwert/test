#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
echo "=== HV3 auth token len ==="
ssh -o BatchMode=yes root@185.84.224.84 "python3 -c \"import json; print('hv3', len(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token']))\"" 2>/dev/null || echo hv3_fail
echo "=== HV5 auth token len ==="
ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json; print('hv5', len(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token']))\"" 2>/dev/null || echo hv5_fail

echo "=== NL curl HV3 health ==="
TOK3=$(ssh -o BatchMode=yes root@185.84.224.84 "python3 -c \"import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" 2>/dev/null)
curl -sk -m 8 "https://185.84.224.84:8892/health" -H "Authorization: Bearer $TOK3" -w "\nHV3_HTTP=%{http_code}\n" | tail -3

echo "=== NL curl HV5 health ==="
TOK5=$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" 2>/dev/null)
curl -sk -m 8 "https://66.151.40.165:8892/health" -H "Authorization: Bearer $TOK5" -w "\nHV5_HTTP=%{http_code}\n" | tail -3

echo "=== Fix HV5 auth with FULL plain token ==="
cd /opt/virtfusion/app/control
/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
$hv5 = Hypervisor::find(5);
$plain = Crypt::decryptString($hv5->token);
$hash = bin2hex(random_bytes(32));
$auth = ['ip' => '66.248.206.14', 'token' => $plain, 'hash' => $hash, 'id' => 5];
file_put_contents('/tmp/hv5-auth-full.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "plain_len=" . strlen($plain) . "\n";
PHP
scp -o BatchMode=yes /tmp/hv5-auth-full.json root@66.151.40.165:/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@66.151.40.165 'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

TOK5=$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" 2>/dev/null)
echo "=== After full token ==="
curl -sk -m 8 "https://66.151.40.165:8892/health" -H "Authorization: Bearer $TOK5" -w "\nHV5_HTTP=%{http_code}\n" | tail -3
curl -sk -m 8 "https://66.151.40.165:8892/hypervisor/resources" -H "Authorization: Bearer $TOK5" -w "\nRES_HTTP=%{http_code}\n" | tail -3

supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL
