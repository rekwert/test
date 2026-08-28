#!/usr/bin/env bash
# Convert DE hypervisor (185.84.224.84) to midrange-only: install VF agent, re-commission, portal node.
# Run on back server (sources .env.probe for SSH passwords).
set -euo pipefail

ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env.probe"
set +a

DE_IP="${DE_IP:-185.84.224.84}"
DE_HOSTNAME="${DE_HOSTNAME:-DE-midrange}"
DE_HV_ID="${DE_HV_ID:-3}"
DE_GROUP_ID="${DE_GROUP_ID:-3}"
DE_NET_ID="${DE_NET_ID:-3}"

ssh_de() {
  SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 "root@${DE_IP}" "$@"
}

ssh_nl() {
  SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 root@66.248.206.14 "$@"
}

echo "=== 1. DE host: storage + VirtFusion hypervisor agent ==="
ssh_de 'bash -s' <<'REMOTE'
set -euo pipefail
mkdir -p /home/vf-data/disk
chmod 755 /home/vf-data /home/vf-data/disk
if [ ! -d /opt/virtfusion/app/hypervisor ]; then
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
  sleep 8
fi
test -d /opt/virtfusion/app/hypervisor && echo VF_AGENT_OK || { echo VF_AGENT_FAIL; exit 1; }
supervisorctl status 2>/dev/null | head -8 || systemctl is-active libvirtd || true
REMOTE

echo "=== 2. NL: ensure SSH key + routed pool on DE ==="
ssh_nl "bash -s" <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='$DE_SSH_PASS'
sshpass -e ssh -o StrictHostKeyChecking=no root@${DE_IP} \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@${DE_IP} hostname

"\${MY[@]}" -e "UPDATE hypervisors SET name='${DE_HOSTNAME}', enabled=1, maintenance=0, commissioned=0 WHERE id=${DE_HV_ID};"
"\${MY[@]}" -e "UPDATE hypervisor_groups SET name='DE Midrange', enabled=1 WHERE id=${DE_GROUP_ID};"

cd /opt/virtfusion/app/control
printf '${DE_HV_ID}\nyes\nyes\n' | \$PHP artisan hypervisor:re-commission 2>&1 | tail -20 || true

"\${MY[@]}" -e "SELECT id,name,ip,commissioned,hypervisor_group_id FROM hypervisors WHERE id=${DE_HV_ID};"
REMOTE

echo "=== 3. Link midrange VF packages to DE group ${DE_GROUP_ID} ==="
if [ -f "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" ]; then
  ssh_nl "bash -s" < "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" 2>&1 | tail -15 || true
fi

echo "=== 4. Portal DB: DE midrange-only node ==="
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"
export POSTGRES_DSN
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes
SET name = 'DE-mid',
    vf_name = 'DE-midrange',
    supported_tiers = ARRAY['midrange']::text[],
    status = 'online',
    vf_enabled = true,
    updated_at = now()
WHERE region = 'de' AND vf_ip = '185.84.224.84';

-- Ensure midrange waitlist can promote (dedicated node without prosto).
INSERT INTO vps.admin_actions (staff_id, user_id, instance_id, action, details)
VALUES (NULL, NULL, NULL, 'de_midrange_node_setup',
  jsonb_build_object('note', 'DE 185.84.224.84 configured midrange-only', 'vf_ip', '185.84.224.84'));

SELECT id, name, vf_name, supported_tiers, external_id, vf_ip, status
FROM vps.nodes WHERE region = 'de';
SQL

echo "DE_MIDRANGE_SETUP_DONE"
