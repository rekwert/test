#!/usr/bin/env bash
# Diagnose FI guest-agent / VF password queue failures.
set -euo pipefail

NL="${NL_HOST:-root@66.248.206.14}"
FI="${FI_HOST:-root@95.216.1.155}"
FI_PASS="${FI_SSH_PASS:-EidNq_riB9F3rD}"

echo "=== VF queue (hypervisor_id=2, FI) ==="
ssh -o BatchMode=yes -o StrictHostKeyChecking=no "$NL" 'bash -s' <<'REMOTE'
source /opt/virtfusion/app/control/.env
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "
SELECT CONCAT('q=',id,' srv=',COALESCE(server_id,''),' act=',action,' fail=',COALESCE(failed,0),' err=',LEFT(COALESCE(error,''),100))
FROM queue WHERE hypervisor_id=2 ORDER BY id DESC LIMIT 12;
"
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "
SELECT id,name,ip,port,enabled,commissioned,maintenance FROM hypervisors WHERE id=2;
"
echo "--- vf-queue-hv errors (NL panel) ---"
supervisorctl tail -4000 vf-queue-hv:00 2>/dev/null | grep -iE 'reset|password|guest|8892|95\.216|error|fail|timeout|respond|connect' | tail -30 || true
echo "--- NL -> FI HV :8892 ---"
curl -sk --connect-timeout 5 -o /dev/null -w "curl_8892_http=%{http_code}\n" https://95.216.1.155:8892/ || echo curl_fail
REMOTE

echo "=== FI hypervisor local ==="
SSHPASS="$FI_PASS" sshpass -e ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "$FI" 'bash -s' <<'REMOTE'
echo "--- supervisor ---"
supervisorctl status 2>/dev/null | head -20 || systemctl list-units 'vf-*' --no-pager 2>/dev/null | head -10
echo "--- guest-agent / queue logs ---"
for f in \
  /opt/virtfusion/app/hypervisor/storage/logs/queue-hv.log \
  /home/vf-data/logs/queue-hv.log \
  /var/log/virtfusion/queue-hv.log; do
  if [[ -f "$f" ]]; then
    echo "file=$f"
    tail -40 "$f" | grep -iE 'reset|password|guest|error|fail|timeout|qemu|agent' || tail -15 "$f"
    break
  fi
done
echo "--- libvirt guest-agent ---"
virsh list --all 2>/dev/null | head -15
for d in $(virsh list --name 2>/dev/null | head -5); do
  [[ -z "$d" ]] && continue
  echo "domain=$d agent=$(virsh qemu-agent-command "$d" '{"execute":"guest-ping"}' 2>&1 | head -1)"
done
echo "--- listen 8892 ---"
ss -tlnp 2>/dev/null | grep 8892 || netstat -tlnp 2>/dev/null | grep 8892 || true
REMOTE
