#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
DE_MID_PASS="$DE_MID_SSH_PASS"

echo "=== 1. Compare detailed versions MID vs PROSTO ==="
for H in "MID:66.151.40.165:$DE_MID_PASS" "PROSTO:185.84.224.84:$DE_SSH_PASS"; do
  IFS=: read -r name ip pass <<< "$H"
  echo "--- $name ---"
  SSHPASS="$pass" sshpass -e ssh -o StrictHostKeyChecking=no root@$ip bash -s <<'R'
cd /opt/virtfusion/app/hypervisor
find . -maxdepth 3 -name 'version' -o -name 'VERSION' -o -name '.version' 2>/dev/null | while read f; do echo "$f: $(cat $f 2>/dev/null)"; done
/opt/virtfusion/php8/bin/php -r '
$j=json_decode(file_get_contents("composer.lock"),true);
foreach($j["packages"]??[] as $p) {
  if(($p["name"]??"")==="virtfusion/global") { echo "virtfusion/global ".$p["version"]."\n"; break; }
}
' 2>/dev/null || true
ls -la storage/app/ 2>/dev/null | head -5
R
done

echo "=== 2. Run hypervisor update on DE-mid ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'MID'
set -euo pipefail
cd /opt/virtfusion/app/hypervisor
bash update 2>&1 | tail -30
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3 || true
/opt/virtfusion/php8/bin/php artisan --version 2>/dev/null
MID

echo "=== 3. NL: cleanup zombie HV5 servers + failed tasks ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
cd /opt/virtfusion/app/control

for SID in 645 647 650 651 652; do
  "${MY[@]}" -e "UPDATE servers SET deleted_at=NOW(), state='failed' WHERE id=$SID AND deleted_at IS NULL;" 2>/dev/null || true
done

# failed server tasks table hunt
for T in failed_server_tasks server_failed_tasks task_failures hypervisor_alerts; do
  "${MY[@]}" -e "SELECT COUNT(*) FROM $T;" 2>/dev/null && echo "table=$T" || true
done

"${MY[@]}" -e "DELETE FROM hypervisor_alerts WHERE hypervisor_id=5;" 2>/dev/null || true
"${MY[@]}" -e "SHOW TABLES LIKE '%alert%';"
"${MY[@]}" -e "SHOW TABLES LIKE '%task%';"

# force-remove zombies if still present
for SID in 645 647 650 651 652; do
  EXISTS=$("${MY[@]}" -N -e "SELECT COUNT(*) FROM servers WHERE id=$SID AND deleted_at IS NULL;")
  if [[ "$EXISTS" != "0" ]]; then
    printf "$SID\nyes\n" | $PHP artisan server:force-delete 2>&1 | tail -3 || true
  fi
done

"${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1, maintenance=0 WHERE id=5;"
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -3 || true
"${MY[@]}" -e "SELECT id,hypervisor_id,state,deleted_at IS NOT NULL del FROM servers WHERE hypervisor_id=5 ORDER BY id;"
NL

echo "=== 4. Re-commission HV5 after update ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(/usr/bin/mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
cd /opt/virtfusion/app/control
export SSHPASS='${DE_MID_PASS}'
printf "5\nyes\nyes\n${DE_MID_PASS}\n" | \$PHP artisan hypervisor:re-commission 2>&1 | tail -10
"\${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) tok FROM hypervisors WHERE id=5;"
REMOTE

echo "=== 5. Agent connectivity test ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
TOK=$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" 2>/dev/null || echo "")
if [ -n "$TOK" ]; then
  code=$(curl -sk -o /tmp/midres -w '%{http_code}' -H "Authorization: Bearer $TOK" "https://66.151.40.165:8892/hypervisor/resources")
  echo "resources HTTP=$code $(head -c 200 /tmp/midres)"
fi
NL

echo "DE_MID_FIX_DONE"
