#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_PASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<NL
set -eo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
export SSHPASS='${DE_PASS}'
printf '5\nyes\nyes\n${DE_PASS}\n' | timeout 240 /opt/virtfusion/php8/bin/php artisan hypervisor:re-commission
mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -e "SELECT id,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
TOK=\$(sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"")
curl -sk -m 15 "https://66.151.40.165:8892/hypervisor/monitor" -H "Authorization: Bearer \$TOK" -w "\nMON=%{http_code}\n" | tail -5
NL
