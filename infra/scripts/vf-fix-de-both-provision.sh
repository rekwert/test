#!/usr/bin/env bash
# Fix DE-prosto (HV3) + DE-midrange (HV5) for VirtFusion provisioning.
# Run on back server (198.13.189.75) as root.
set -euo pipefail

ROOT="${ROOT:-/opt/testVPStrade}"
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"

DE_MID_PASS="${DE_MID_SSH_PASS:?set DE_MID_SSH_PASS in .env.probe}"

echo "=== 0. Current portal state ==="
psql "$POSTGRES_DSN" -c \
  "SELECT o.order_number, i.id, i.hostname, i.state, i.ip_address, i.external_id, n.name
   FROM vps.instances i
   JOIN vps.orders o ON o.id = i.order_id
   LEFT JOIN vps.nodes n ON n.id = i.node_id
   WHERE n.region = 'de' ORDER BY i.id DESC LIMIT 10;"

echo "=== 1. VF NL: cleanup + commission HV5 (GB pattern) ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N)
cd /opt/virtfusion/app/control

echo "--- Soft-delete failed VF servers on HV5 ---"
for SID in 645 647 649; do
  "\${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE id=\$SID AND deleted_at IS NULL;" 2>/dev/null || true
done
"\${MY[@]}" -e "SELECT id,state,deleted_at FROM servers WHERE id IN (645,647,649);"

echo "--- Ensure NL SSH key on DE-mid ---"
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=\$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS='${DE_MID_PASS}'
ssh-keygen -f /root/.ssh/known_hosts -R '66.151.40.165' 2>/dev/null || true
sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  "grep -qxF '\$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '\$PUB' >> /root/.ssh/authorized_keys"
ssh -o BatchMode=yes -o ConnectTimeout=15 root@66.151.40.165 hostname

echo "--- HV5 storage/network/packages ---"
HV=5
ST=\$("\${MYN[@]}" -e "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=\$HV;")
if [[ "\$ST" == "0" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_storage (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES (\$HV,'Local disk','/home/vf-data/disk','mountpoint',8000,0,1,1,'[]',NOW(),NOW());"
fi
NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=\$HV AND \`primary\`=1 LIMIT 1;")
if [[ -z "\${NET:-}" ]]; then
  "\${MY[@]}" -e "INSERT INTO hypervisor_networks (hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled,created_at,updated_at)
    VALUES (\$HV,'simpleBridge','br0',1,1,1,NOW(),NOW());"
  NET=\$("\${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=\$HV AND \`primary\`=1 LIMIT 1;")
fi
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,\$HV);"
"\${MY[@]}" -e "INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,\$NET,\$HV);"
for PKG in \$(mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N -e "SELECT id FROM server_packages WHERE enabled=1;"); do
  "\${MY[@]}" -e "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id) VALUES (5,\$PKG);" 2>/dev/null || true
done

echo "--- Keep HV3 commissioned=3 (do not reset) ---"
"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1, maintenance=0, prohibit=0 WHERE id=3;"

echo "--- Re-commission HV5 with SSH password ---"
"\${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, enabled=1, maintenance=0, prohibit=0, token=NULL WHERE id=5;"
printf "5\nyes\nyes\n${DE_MID_PASS}\n" | \$PHP artisan hypervisor:re-commission 2>&1 | tail -40

COMM=0
TOK_LEN=0
for i in \$(seq 1 18); do
  sleep 10
  COMM=\$("\${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=5;")
  TOK_LEN=\$("\${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=5;")
  echo "poll \$i HV5 commissioned=\$COMM token_len=\$TOK_LEN"
  [[ "\$COMM" == "3" && "\$TOK_LEN" -gt 100 ]] && break
done

AUTH=\$(ssh -o BatchMode=yes -o ConnectTimeout=10 root@66.151.40.165 \
  'test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo MISSING' 2>/dev/null || echo SSH_FAIL)
echo "DE-mid auth.json: \$AUTH"

if [[ "\$COMM" != "3" || "\$TOK_LEN" -lt 100 ]]; then
  echo "--- Commission incomplete; check allocation log ---"
  ACC=\$("\${MYN[@]}" -e "SELECT accepted FROM log_resource_allocation ORDER BY id DESC LIMIT 1;" 2>/dev/null || echo 0)
  echo "last allocation accepted=\$ACC"
  if [[ "\$AUTH" == "OK" && "\$TOK_LEN" -gt 100 ]]; then
    "\${MY[@]}" -e "UPDATE hypervisors SET commissioned=3 WHERE id=5;"
    echo "forced HV5 commissioned=3 (auth.json + token present)"
  fi
fi

echo "--- Final HV3/HV5 ---"
"\${MY[@]}" -e "SELECT id,name,ip,enabled,commissioned,LENGTH(token) tok_len FROM hypervisors WHERE id IN (3,5);"

supervisorctl restart vf-queue-hv: vf-queue-control: 2>/dev/null | tail -5 || true
REMOTE

echo "=== 2. Restart DE-mid vf-queue on host ==="
SSHPASS="$DE_MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  "supervisorctl restart vf-queue-hv: 2>/dev/null | tail -3 || true; \
   test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo auth.json=OK || echo auth.json=MISSING"

echo "=== 3. Reset stuck DE-mid instance #199 if VF build failed ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET external_id = NULL,
    ip_address = NULL,
    state = 'creating',
    updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number = 199 AND i.state IN ('creating', 'error')
);
SQL

echo "=== 4. Restart worker (clear backoff) ==="
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "=== 5. Wait 90s for worker poll ==="
sleep 90

echo "=== 6. Worker logs ==="
docker logs docker-vps-worker-1 --since 2m 2>&1 | grep -iE 'retry allocate|complete provision|provision guest|enough resources|not commissioned|401|198|199' | tail -25 || true

echo "=== 7. Final portal ==="
psql "$POSTGRES_DSN" -c \
  "SELECT o.order_number, i.hostname, i.state, i.ip_address, i.external_id, n.name
   FROM vps.instances i
   JOIN vps.orders o ON o.id = i.order_id
   LEFT JOIN vps.nodes n ON n.id = i.node_id
   WHERE o.order_number IN (198,199) OR (n.region='de' AND i.created_at > now() - interval '2 hours');"

echo "DE_BOTH_FIX_DONE"
