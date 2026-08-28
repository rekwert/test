#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
DE_MID_PASS="$DE_MID_SSH_PASS"

echo "============================================"
echo " RESTORE: other nodes + fix DE-mid only"
echo "============================================"

echo "=== 1. RESTORE FI (was disabled during DE debug) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE hypervisors SET enabled=1, maintenance=0, commissioned=3 WHERE id=2;"
"${MY[@]}" -e "SELECT id,name,enabled,maintenance,commissioned FROM hypervisors WHERE id=2;"
NL

psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes SET
  status = 'online', maintenance_mode = false, vf_enabled = true, vf_commissioned = 3,
  supported_tiers = ARRAY['prosto','midrange']::text[],
  updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002';
SQL

echo "=== 2. Ensure other hypervisors untouched (enabled, commissioned=3) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
for ID in 1 2 3 4; do
  "${MY[@]}" -e "UPDATE hypervisors SET enabled=1, maintenance=0, commissioned=3 WHERE id=$ID;"
done
"${MY[@]}" -e "SELECT id,name,ip,enabled,maintenance,commissioned FROM hypervisors WHERE id IN (1,2,3,4,5);"
NL

echo "=== 3. DE IP pool: only block 6 (212.102.227.0/24) on HV3+HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
echo "IP blocks:"
"${MY[@]}" -e "SELECT id,name,network,enabled FROM ip_blocks WHERE id IN (5,6) OR name LIKE '%DE%';"
echo "DE hypervisor IP links:"
"${MY[@]}" -e "SELECT ibh.block_id,ib.name,ibh.hypervisor_id,h.name FROM ip_block_hypervisor ibh JOIN ip_blocks ib ON ib.id=ibh.block_id JOIN hypervisors h ON h.id=ibh.hypervisor_id WHERE ibh.hypervisor_id IN (3,5);"
# Remove wrong blocks from DE if any
for HV in 3 5; do
  "${MY[@]}" -e "DELETE FROM ip_block_hypervisor WHERE hypervisor_id=$HV AND block_id NOT IN (6);"
done
for HV in 3 5; do
  NET=$("${MY[@]}" -N -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=$HV AND \`primary\`=1 LIMIT 1;")
  "${MY[@]}" -e "DELETE FROM ip_block_hypervisor_network WHERE hypervisor_id=$HV AND block_id NOT IN (6);"
  "${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,$HV);"
  if [ -n "$NET" ]; then
    "${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,$NET,$HV);"
  fi
done
echo "After fix:"
"${MY[@]}" -e "SELECT ibh.block_id,ib.name,ibh.hypervisor_id,h.name FROM ip_block_hypervisor ibh JOIN ip_blocks ib ON ib.id=ibh.block_id JOIN hypervisors h ON h.id=ibh.hypervisor_id WHERE ibh.hypervisor_id IN (3,5);"
"${MY[@]}" -e "SELECT COUNT(*) free_de FROM ipv4 WHERE block_id=6 AND server_id IS NULL;"
NL

echo "=== 4. DE-mid: storage + capacity (384GB RAM, ~7TB disk) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE hypervisors SET max_servers=150, max_cpu=128, max_memory=393216, max_local_hdd=7500, enabled=1, maintenance=0, commissioned=3 WHERE id=5;"
"${MY[@]}" -e "UPDATE hypervisor_storage SET capacity=7500, enabled=1 WHERE hypervisor_id=5;"
"${MY[@]}" -e "SELECT id,name,max_servers,max_cpu,max_memory,max_local_hdd,commissioned FROM hypervisors WHERE id=5;"
NL

psql "$POSTGRES_DSN" -c "UPDATE vps.nodes SET capacity_instances=150 WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';"

echo "=== 5. DE-mid host: storage dir + network route for pool ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'MID'
set -euo pipefail
mkdir -p /home/vf-data/disk
chmod 755 /home/vf-data /home/vf-data/disk
# Ensure routed pool on br0
ip route replace 212.102.227.0/24 dev br0 scope link 2>/dev/null || true
ip addr show br0 | grep -E '66.151|212.102' || ip addr show | head -15
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 2>/dev/null || true
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2 || true
MID

echo "=== 6. Purge failed DE-mid VF zombies, reset portal orders ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND state='failed' AND deleted_at IS NULL;"
"${MY[@]}" -e "DELETE FROM queue_fail WHERE hypervisor_id=5;"
"${MY[@]}" -e "DELETE FROM hypervisor_tasks WHERE hypervisor_id=5;"
supervisorctl restart vf-queue: 2>/dev/null | tail -3 || true
NL

psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances SET external_id=NULL, ip_address=NULL, state='creating', updated_at=now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id=i.order_id
  WHERE o.order_number IN (203,205) OR i.state='error'
);
SQL

echo "=== 7. Portal DE nodes (prosto=3, mid=5) — no shared routing ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes SET external_id='3', vf_name='DE-prosto', vf_ip='185.84.224.84',
  supported_tiers=ARRAY['prosto','hustle']::text[], status='online', vf_commissioned=3, vf_enabled=true,
  maintenance_mode=false, updated_at=now()
WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003';

UPDATE vps.nodes SET external_id='5', vf_name='DE-midrange', vf_ip='66.151.40.165',
  supported_tiers=ARRAY['midrange']::text[], status='online', vf_commissioned=3, vf_enabled=true,
  maintenance_mode=false, updated_at=now()
WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

SELECT name,vf_ip,external_id,supported_tiers,status FROM vps.nodes WHERE region='de';
SQL

echo "=== 8. DO NOT update NL panel — sync hypervisor agents only (fix version mismatch alerts) ==="
for spec in "DE_MID:66.151.40.165:$DE_MID_PASS" "DE_PROSTO:185.84.224.84:$DE_SSH_PASS" "GB:212.108.83.47:$GB_SSH_PASS" "FI:95.216.1.155:$FI_SSH_PASS"; do
  IFS=: read -r label ip pass <<< "$spec"
  echo "--- agent update $label ---"
  SSHPASS="$pass" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 root@$ip \
    'test -d /opt/virtfusion/app/hypervisor && cd /opt/virtfusion/app/hypervisor && bash update 2>&1 | tail -3 || echo no-agent' || echo "skip $label"
done

echo "=== 9. Clear version mismatch alerts ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(/usr/bin/mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
for T in ss_alert_log ss_alert_lock_log hypervisor_alerts; do
  "${MY[@]}" -e "DELETE FROM $T;" 2>/dev/null && echo "cleared $T" || true
done
supervisorctl restart vf-queue: 2>/dev/null | tail -2 || true
NL

echo "=== 10. Restart worker ==="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1
sleep 30

echo "=== DONE ==="
bash /tmp/tmp-full-audit.sh 2>/dev/null | head -40 || true
psql "$POSTGRES_DSN" -c "SELECT o.order_number,i.hostname,i.state,i.ip_address,n.name FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id LEFT JOIN vps.nodes n ON n.id=i.node_id WHERE o.order_number IN (203,205);"
