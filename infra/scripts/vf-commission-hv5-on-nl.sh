#!/usr/bin/env bash
# Run ON VirtFusion NL panel (66.248.206.14) as root.
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)
DE_MID_IP="${DE_MID_IP:-66.151.40.165}"
DE_MID_PASS="${DE_MID_PASS:?set DE_MID_PASS}"
cd /opt/virtfusion/app/control

echo "=== SSH key to DE-mid ==="
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS="$DE_MID_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_MID_IP" \
  "grep -qxF '$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '$PUB' >> /root/.ssh/authorized_keys" || true
ssh -o BatchMode=yes -o ConnectTimeout=15 root@"$DE_MID_IP" hostname || { echo SSH_FAIL; exit 1; }

echo "=== SS group 5 ==="
EXISTS=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")
if [[ "$EXISTS" == "0" ]]; then
  NEXT=$("${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES ($NEXT, 5, 'DE Mid', 'DE Mid', 5);"
fi
GID=$("${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
EXISTS2=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;")
if [[ "$EXISTS2" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 5, $GID, 5);"
fi

echo "=== Settings/config HV5 ==="
ST=$("${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_settings WHERE hypervisor_id=5;")
if [[ "$ST" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO hypervisor_settings (hypervisor_id, force_ipv6, timezone, vnc_listen_type, display_name, cpu_set, disk_driver_io, created_at, updated_at)
    SELECT 5, force_ipv6, timezone, vnc_listen_type, display_name, cpu_set, disk_driver_io, NOW(), NOW() FROM hypervisor_settings WHERE hypervisor_id=3;"
  "${MY[@]}" -e "INSERT INTO hypervisor_config (hypervisor_id, \`key\`, value, created_at, updated_at)
    SELECT 5, \`key\`, value, NOW(), NOW() FROM hypervisor_config WHERE hypervisor_id=3;"
fi

echo "=== Soft-delete failed HV5 servers ==="
"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND state='failed' AND deleted_at IS NULL;"

echo "=== Reset + re-commission HV5 ==="
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, enabled=1, maintenance=0, prohibit=0, token=NULL WHERE id=5;"
ssh -o BatchMode=yes root@"$DE_MID_IP" 'rm -f /opt/virtfusion/app/hypervisor/conf/auth.json' || true

# Try with pseudo-TTY via script if available
COMM_CMD="printf '5\nyes\nyes\n${DE_MID_PASS}\n' | $PHP artisan hypervisor:re-commission"
if command -v script >/dev/null; then
  script -q -c "$COMM_CMD" /dev/null 2>&1 | tee /tmp/hv5-comm.log
else
  eval "$COMM_CMD" 2>&1 | tee /tmp/hv5-comm.log
fi

COMM=0; TOK=0
for i in $(seq 1 30); do
  sleep 10
  COMM=$("${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
  TOK=$("${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
  AUTH=$(ssh -o BatchMode=yes root@"$DE_MID_IP" 'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO' 2>/dev/null)
  echo "poll $i commissioned=$COMM token_len=$TOK auth=$AUTH"
  [[ "$COMM" == "3" && "$TOK" -gt 500 && "$AUTH" == "OK" ]] && break
done

echo "=== Commission log tail ==="
tail -25 /tmp/hv5-comm.log 2>/dev/null || true

if [[ "$COMM" != "3" || "$TOK" -lt 500 || "$AUTH" != "OK" ]]; then
  echo "COMMISSION_FAILED commissioned=$COMM token=$TOK auth=$AUTH"
  "${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,prohibit,maintenance FROM hypervisors WHERE id=5\G"
  exit 1
fi

"${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1 WHERE id=3;"
"${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) tok FROM hypervisors WHERE id IN (3,5);"
supervisorctl restart vf-queue: vf-queue-hv: 2>/dev/null | tail -5 || true
ssh -o BatchMode=yes root@"$DE_MID_IP" 'supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2' || true
echo NL_COMMISSION_OK
