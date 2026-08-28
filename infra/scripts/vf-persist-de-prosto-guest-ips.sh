#!/usr/bin/env bash
# Persist static IP in all DE-prosto guests after reserved-IP migration.
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

DE_IP=212.102.227.6
GW=212.102.227.1
PFX=24
HELPER=/tmp/guest-persist-ip-de.sh
REMOTE=$HELPER

declare -A VM_IP=(
  [597]=212.102.227.43
  [599]=212.102.227.44
  [517]=212.102.227.45
  [551]=212.102.227.41
  [553]=212.102.227.42
)

UUID_MAP=$(SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e \
  "SELECT id,uuid,name FROM servers WHERE id IN (597,599,517,551,553) ORDER BY id;"
NL
)

SSHPASS="$DE_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no "$HELPER" root@"$DE_IP":"$REMOTE"
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" "chmod +x $REMOTE; sed -i 's/\r$//' $REMOTE"

echo "=== Persist guest static IP ==="
FAIL=0
mapfile -t ROWS <<< "$UUID_MAP"
for row in "${ROWS[@]}"; do
  [ -z "$row" ] && continue
  read -r sid uuid name <<< "$row"
  [ -z "$sid" ] && continue
  ip="${VM_IP[$sid]:-}"
  [ -n "$ip" ] || continue
  echo "--- $name ($sid) -> $ip ---"
  if ! SSHPASS="$DE_SSH_PASS" sshpass -e ssh -n -o StrictHostKeyChecking=no root@"$DE_IP" \
    "$REMOTE" "$uuid" "$ip" "$GW" "$PFX"; then
    FAIL=1
  fi
done

echo "=== Ping check ==="
for ip in 212.102.227.41 212.102.227.42 212.102.227.43 212.102.227.44 212.102.227.45; do
  ping -c1 -W2 "$ip" >/dev/null 2>&1 && echo "$ip OK" || echo "$ip FAIL"
done

[ "$FAIL" -eq 0 ] && echo PERSIST_DONE || { echo PERSIST_PARTIAL; exit 1; }
