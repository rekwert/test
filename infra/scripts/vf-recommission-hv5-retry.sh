#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_PASS="${DE_MID_SSH_PASS:?DE_MID_SSH_PASS is required}"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<NL
set -eo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")

echo "=== before HV5 ==="
"\${MY[@]}" -e "SELECT id,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"

echo "=== ensure SSH key on DE-mid ==="
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_PASS}'
ssh-keygen -f /root/.ssh/known_hosts -R '66.151.40.165' 2>/dev/null || true
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@66.151.40.165 hostname

"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, enabled=1, maintenance=0, prohibit=0, token=NULL WHERE id=5;"

echo "=== artisan re-commission ==="
printf '5\nyes\nyes\n${DE_PASS}\n' | timeout 180 /opt/virtfusion/php8/bin/php artisan hypervisor:re-commission 2>&1 | tail -50

echo "=== after HV5 ==="
"\${MY[@]}" -e "SELECT id,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
AUTH=\$(ssh -o BatchMode=yes root@66.151.40.165 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO')
TOKLEN=\$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json;print(len(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token']))\"" 2>/dev/null || echo 0)
echo "auth=\$AUTH toklen=\$TOKLEN"

TOK=\$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" )
curl -sk -m 12 "https://66.151.40.165:8892/hypervisor/monitor" -H "Authorization: Bearer \$TOK" -w "\nMON=%{http_code}\n" | tail -5

ssh -o BatchMode=yes root@66.151.40.165 'systemctl start libvirtd; systemctl start vf-nginx; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'
supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL

echo "=== retry gm4kgz ==="
source /opt/testVPStrade/infra/docker/.env
SSHPASS="$NL_SSH_PASS" sshpass -e ssh root@66.248.206.14 "mysql -h127.0.0.1 -u\$(grep DB_USERNAME /opt/virtfusion/app/control/.env|cut -d= -f2) -p\$(grep DB_PASSWORD /opt/virtfusion/app/control/.env|cut -d= -f2) \$(grep DB_DATABASE /opt/virtfusion/app/control/.env|cut -d= -f2) -e \"UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL AND state IN ('failed','allocated');\""

psql "$POSTGRES_DSN" -c "UPDATE vps.instances SET external_id=NULL, ip_address=NULL, state='creating', updated_at=now() WHERE hostname='vps-gm4kgz';"
docker restart docker-vps-worker-1
sleep 120
bash /tmp/check-gm4kgz.sh 2>/dev/null || true
