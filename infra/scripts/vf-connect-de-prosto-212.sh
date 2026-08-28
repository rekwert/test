#!/usr/bin/env bash
# Re-bind DE-prosto (HV3) to 212.102.227.6 in VirtFusion + portal only.
# Does NOT reboot, does NOT touch libvirt/VMs, does NOT re-commission.
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env
source /opt/testVPStrade/infra/docker/.env.probe

PROSTO_IP=212.102.227.6

echo "=== 1. NL SSH trust to DE-prosto (for VF connectivity checks) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o ConnectTimeout=12 -o StrictHostKeyChecking=no root@66.248.206.14 'bash -s' <<REMOTE
set -euo pipefail
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_SSH_PASS}'
ssh-keygen -f /root/.ssh/known_hosts -R '${PROSTO_IP}' >/dev/null 2>&1 || true
sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@${PROSTO_IP} \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
unset SSHPASS
ssh-keyscan -H ${PROSTO_IP} >> /root/.ssh/known_hosts 2>/dev/null
ssh -o BatchMode=yes -o ConnectTimeout=10 root@${PROSTO_IP} 'echo PROSTO_SSH_OK; virsh list --name | wc -l'
REMOTE

echo "=== 2. VirtFusion: point HV3 to new IP (DB only) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o ConnectTimeout=12 -o StrictHostKeyChecking=no root@66.248.206.14 'bash -s' <<'REMOTE'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" <<'SQL'
UPDATE hypervisors
SET ip = '212.102.227.6',
    port = 8892,
    ssh_port = 22,
    enabled = 1,
    maintenance = 0,
    prohibit = 0
WHERE id = 3;

SELECT id,name,ip,port,commissioned,enabled,maintenance,LENGTH(token) token_len
FROM hypervisors WHERE id=3;

SELECT COUNT(*) running_vms FROM servers
WHERE hypervisor_id=3 AND deleted_at IS NULL;
SQL
supervisorctl restart vf-queue-hv: vf-queue-control: >/dev/null 2>&1 || true
REMOTE

echo "=== 3. Portal node DE-1 ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes
SET vf_ip = '212.102.227.6',
    status = 'online',
    vf_enabled = true,
    vf_commissioned = 3,
    maintenance_mode = false,
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003'::uuid;

SELECT id,name,vf_ip,status,external_id,maintenance_mode
FROM vps.nodes WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003'::uuid;
SQL

echo "=== 4. Verify from NL (no host changes) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o ConnectTimeout=12 -o StrictHostKeyChecking=no root@66.248.206.14 \
  'nc -zvw5 212.102.227.6 8892; ssh -o BatchMode=yes root@212.102.227.6 "virsh list --state-running | grep running | wc -l"'

echo "DE_PROSTO_CONNECTED"
