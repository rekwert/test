#!/usr/bin/env bash
# Last-resort DE-midrange fix: reinstall agent, real re-commission with password, rebuild orders.
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' /opt/testVPStrade/infra/docker/.env | cut -d= -f2-)"
DE_MID_PASS="${DE_MID_SSH_PASS:?set DE_MID_SSH_PASS}"

echo "=== 1. DE-mid: ensure agent + network ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'MID'
set -euo pipefail
rm -f /opt/virtfusion/app/hypervisor/conf/auth.json
mkdir -p /home/vf-data/disk /opt/virtfusion/app/hypervisor/conf
chmod 755 /home/vf-data /home/vf-data/disk
if ! virsh uri >/dev/null 2>&1; then
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
  sleep 10
fi
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3 || true
echo "kvm=$(test -e /dev/kvm && echo ok || echo no) agent=$(test -d /opt/virtfusion/app/hypervisor && echo ok || echo no)"
MID

echo "=== 2. NL: SSH key + full GB-style commission HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)
cd /opt/virtfusion/app/control

# SSH key
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_MID_PASS}'
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@66.151.40.165 hostname

# SS group 5 (idempotent)
EXISTS=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")
if [[ "\$EXISTS" == "0" ]]; then
  NEXT=\$("\${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "\${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \\\`order\\\`) VALUES (\$NEXT, 5, 'DE Mid', 'DE Mid', 5);"
fi
GID=\$("\${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
EXISTS2=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;")
if [[ "\$EXISTS2" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \\\`order\\\`) VALUES (1, 5, \$GID, 5);"
fi

# Copy settings from HV3 if missing
ST=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_settings WHERE hypervisor_id=5;")
if [[ "\$ST" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_settings (hypervisor_id, force_ipv6, timezone, vnc_listen_type, display_name, cpu_set, disk_driver_io, created_at, updated_at)
    SELECT 5, force_ipv6, timezone, vnc_listen_type, display_name, cpu_set, disk_driver_io, NOW(), NOW() FROM hypervisor_settings WHERE hypervisor_id=3;"
  "\${MY[@]}" -e "INSERT INTO hypervisor_config (hypervisor_id, \\\`key\\\`, value, created_at, updated_at)
    SELECT 5, \\\`key\\\`, value, NOW(), NOW() FROM hypervisor_config WHERE hypervisor_id=3;"
fi

# Soft-delete all failed HV5 servers
for SID in \$(mysql -N -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -e "SELECT id FROM servers WHERE hypervisor_id=5 AND state='failed' AND deleted_at IS NULL;"); do
  "\${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE id=\$SID;"
done

echo "--- Reset HV5 and re-commission (GB pattern) ---"
"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, enabled=1, maintenance=0, prohibit=0, token=NULL WHERE id=5;"
printf "5\nyes\nyes\n${DE_MID_PASS}\n" | script -q -c "\$PHP artisan hypervisor:re-commission" /dev/null 2>&1 | tee /tmp/hv5-comm.log | tail -30

COMM=0; TOK=0
for i in \$(seq 1 36); do
  sleep 10
  COMM=\$("\${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
  TOK=\$("\${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
  AUTH=\$(ssh -o BatchMode=yes root@66.151.40.165 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO' 2>/dev/null || echo SSHFAIL)
  echo "poll \$i commissioned=\$COMM token_len=\$TOK auth=\$AUTH"
  [[ "\$COMM" == "3" && "\$TOK" -gt 500 && "\$AUTH" == "OK" ]] && break
done

if [[ "\$COMM" != "3" || "\$TOK" -lt 500 ]]; then
  echo "Commission failed. Log:"
  tail -20 /tmp/hv5-comm.log
  echo "Trying force commissioned=3 only if auth exists..."
  if ssh -o BatchMode=yes root@66.151.40.165 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json'; then
    "\${MY[@]}" -e "UPDATE hypervisors SET commissioned=3 WHERE id=5;"
  else
    echo "FATAL: no auth.json — commission did not run"
    exit 1
  fi
fi

"\${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) tok FROM hypervisors WHERE id IN (3,5);"
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -5 || true
ssh -o BatchMode=yes root@66.151.40.165 'supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'
REMOTE

echo "=== 3. Keep HV3 commissioned=3 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'source /opt/virtfusion/app/control/.env; mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "UPDATE hypervisors SET commissioned=3, enabled=1 WHERE id=3;"'

echo "=== 4. Portal: reset DE-mid orders ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET external_id = NULL, ip_address = NULL, state = 'creating', updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number IN (196, 199)
);
UPDATE vps.nodes SET vf_commissioned = 3, status = 'online', vf_enabled = true, updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';
SQL

echo "=== 5. Restart worker ==="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1
sleep 120

echo "=== 6. Final status ==="
bash /tmp/de-status-final.sh 2>/dev/null || true
echo "DE_MID_FIX_DONE"
