#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "DE_PASS='$DE_SSH_PASS' bash -s" <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
export SSHPASS="$DE_PASS"

install -d -m 700 /root/.ssh
if [[ ! -f /root/.ssh/id_ed25519 ]]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(< /root/.ssh/id_ed25519.pub)
sshpass -e ssh -n -o StrictHostKeyChecking=no root@185.84.224.84 \
  "install -d -m 700 /root/.ssh; touch /root/.ssh/authorized_keys; grep -qxF '$PUB' /root/.ssh/authorized_keys || echo '$PUB' >> /root/.ssh/authorized_keys; chmod 600 /root/.ssh/authorized_keys"
ssh -n -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=no root@185.84.224.84 hostname

echo "=== HV3 before ==="
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) token_len FROM hypervisors WHERE id=3;"
echo "=== Reset only HV3 authentication state ==="
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, token=NULL WHERE id=3;"
ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no root@185.84.224.84 \
  "rm -f /opt/virtfusion/app/hypervisor/conf/auth.json"
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) token_len FROM hypervisors WHERE id=3;"
echo "DE_PROSTO_READY_FOR_OFFICIAL_COMMISSION"
NL
