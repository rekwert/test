#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
DE_PASS="${DE_MID_SSH_PASS:?missing DE_MID_SSH_PASS}"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "DE_PASS='$DE_PASS' bash -s" <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
cd /opt/virtfusion/app/control
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)
export SSHPASS="$DE_PASS"

echo "=== Ensure NL can SSH directly to HV5 ==="
install -d -m 700 /root/.ssh
if [[ ! -f /root/.ssh/id_ed25519 ]]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(< /root/.ssh/id_ed25519.pub)
sshpass -e ssh -n -o StrictHostKeyChecking=no root@66.151.40.165 \
  "install -d -m 700 /root/.ssh; touch /root/.ssh/authorized_keys; grep -qxF '$PUB' /root/.ssh/authorized_keys || echo '$PUB' >> /root/.ssh/authorized_keys; chmod 600 /root/.ssh/authorized_keys"
ssh -n -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=no root@66.151.40.165 hostname

echo "=== Reset commission state for HV5 only ==="
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, token=NULL WHERE id=5;"
ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no root@66.151.40.165 \
  "rm -f /opt/virtfusion/app/hypervisor/conf/auth.json"

echo "=== Official VirtFusion re-commission HV5 ==="
set +e
printf "5\nyes\nyes\n%s\n" "$DE_PASS" |
  timeout 600 script -qec "$PHP artisan hypervisor:re-commission -vvv" /dev/null
STATUS=$?
set -e
echo "recommission_status=$STATUS"

echo "=== Poll official result ==="
COMM=0
TOKLEN=0
AUTH=NO
for I in $(seq 1 36); do
  COMM=$("${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
  TOKLEN=$("${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
  AUTH=$(ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no root@66.151.40.165 \
    "test -s /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO")
  echo "poll=$I commissioned=$COMM encrypted_token_len=$TOKLEN auth_json=$AUTH"
  if [[ "$COMM" == "3" && "$TOKLEN" -gt 100 && "$AUTH" == "OK" ]]; then
    break
  fi
  sleep 10
done

"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) token_len FROM hypervisors WHERE id IN (3,5);"
if [[ "$COMM" != "3" || "$TOKLEN" -le 100 || "$AUTH" != "OK" ]]; then
  echo "OFFICIAL_RECOMMISSION_FAILED"
  exit 1
fi

echo "=== Start HV5 services and queues ==="
ssh -o BatchMode=yes -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'HV5'
set -euo pipefail
systemctl enable --now libvirtd vf-nginx
supervisorctl restart vf-queue-hv: || true
systemctl is-active libvirtd vf-nginx
ss -tlnp | python3 -c 'import sys; print("".join(x for x in sys.stdin if ":8892" in x), end="")'
HV5
supervisorctl restart vf-queue: || true
echo "OFFICIAL_RECOMMISSION_OK"
NL
