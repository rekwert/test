#!/bin/bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
DE_MID_PASS="${DE_MID_SSH_PASS:?}"

echo "=== NL: HV5 + server 669 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
"\${MY[@]}" -e "SELECT id,name,ip,enabled,commissioned,maintenance FROM hypervisors WHERE id IN (3,5);"
"\${MY[@]}" -e "SELECT id,hypervisor_id,commission_status,state,built,build_failed,hostname,deleted_at FROM servers WHERE id=669\\G"
"\${MY[@]}" -e "SELECT id,server_id,action,status,message,created_at FROM server_action_logs WHERE server_id=669 ORDER BY id DESC LIMIT 15;" 2>/dev/null || \
"\${MY[@]}" -e "SHOW TABLES LIKE '%log%';" 2>/dev/null | head -20
REMOTE

echo ""
echo "=== DE-mid: services + virsh + logs ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "ssh -o BatchMode=yes -o ConnectTimeout=15 root@66.151.40.165 bash -s" <<'INNER'
LOG=/opt/virtfusion/app/hypervisor/storage/logs/app-$(date +%Y-%m-%d).log
echo services: $(systemctl is-active supervisor libvirtd 2>/dev/null | tr '\n' ' ')
supervisorctl status 2>/dev/null | head -8
echo auth: $(test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo MISSING)
virsh list --all 2>/dev/null | head -15
echo "--- grep 669 ---"
grep 669 "$LOG" 2>/dev/null | tail -25 || echo none
echo "--- errors tail ---"
grep -iE 'error|fail|exception|commission|storage|libvirt|build|f98773a8' "$LOG" 2>/dev/null | tail -35
INNER
