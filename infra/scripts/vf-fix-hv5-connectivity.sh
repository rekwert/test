#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a

echo "=== SSH DE-mid ==="
ssh -o BatchMode=yes -o ConnectTimeout=10 root@66.151.40.165 hostname || echo SSH_FAIL

echo "=== services DE-mid ==="
ssh -o BatchMode=yes root@66.151.40.165 "systemctl is-active libvirtd vf-nginx 2>/dev/null; ss -tlnp | grep 8892 || netstat -tlnp 2>/dev/null | grep 8892" || true

echo "=== regenerate full auth ==="
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
$auth = ['ip' => '66.248.206.14', 'token' => substr($plain,0,200), 'hash' => bin2hex(random_bytes(32)), 'id' => 5];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "plain=".strlen($plain)."\n";
PHP
scp -o BatchMode=yes /tmp/hv5-auth.json root@66.151.40.165:/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@66.151.40.165 'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl start libvirtd; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

echo "=== connectivity from NL to DE-mid:8892 ==="
nc -zv -w5 66.151.40.165 8892 2>&1 || true
TOK=$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" )
curl -sk -m 10 "https://66.151.40.165:8892/hypervisor/resources" -H "Authorization: Bearer $TOK" -w "\nRES=%{http_code}\n" | tail -5

echo "=== test build job - soft delete 676, retry ==="
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "UPDATE servers SET deleted_at=NOW() WHERE id=676 AND deleted_at IS NULL;"
supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL

source /opt/testVPStrade/infra/docker/.env
psql "$POSTGRES_DSN" -c "UPDATE vps.instances SET external_id=NULL, ip_address=NULL, state='creating', updated_at=now() WHERE hostname='vps-gm4kgz';"
docker restart docker-vps-worker-1
sleep 90
psql "$POSTGRES_DSN" -c "SELECT hostname,state,external_id,ip_address::text FROM vps.instances WHERE hostname='vps-gm4kgz';"
docker logs docker-vps-worker-1 --since 2m 2>&1 | grep -iE 'gm4kgz|8d8a46f7|retry|complete provision|676|677|678|679|680' | tail -25
