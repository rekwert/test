#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_PASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"

echo "=== Run re-commission on NL panel ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<NL
set -eo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
export SSHPASS='${DE_PASS}'

echo "=== test DE-mid ssh ==="
sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=15 root@66.151.40.165 hostname

"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, enabled=1, maintenance=0, prohibit=0, token=NULL WHERE id=5;"

echo "=== re-commission HV5 ==="
printf '5\nyes\nyes\n${DE_PASS}\n' | timeout 240 /opt/virtfusion/php8/bin/php artisan hypervisor:re-commission 2>&1 | tail -60

"\${MY[@]}" -e "SELECT id,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 'systemctl enable --now libvirtd vf-nginx; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

# Test monitor with token from auth.json
TOK=\$(sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"")
echo "tok_len=\${#TOK}"
curl -sk -m 15 "https://66.151.40.165:8892/hypervisor/monitor" -H "Authorization: Bearer \$TOK" -w "\nMONITOR=%{http_code}\n" | tail -8

# Test from control using Laravel (same as build job)
/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
\$app = require __DIR__ . '/bootstrap/app.php';
\$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
\$hv = App\Models\Hypervisor::find(5);
try {
    \$client = \$hv->client ?? \$hv->connection ?? null;
    echo "client_class=" . (is_object(\$client) ? get_class(\$client) : 'none') . "\n";
} catch (Throwable \$e) {
    echo "err: " . \$e->getMessage() . "\n";
}
PHP

supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL

echo "=== Clean failed shells + retry gm4kgz ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL2'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" \
  -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL AND id>=669;"
NL2

source /opt/testVPStrade/infra/docker/.env
psql "$POSTGRES_DSN" -c "UPDATE vps.instances SET external_id=NULL, ip_address=NULL, state='creating', updated_at=now() WHERE hostname='vps-gm4kgz';"
docker restart docker-vps-worker-1
sleep 150
sed -i 's/\r$//' /tmp/check-gm4kgz.sh
bash /tmp/check-gm4kgz.sh
