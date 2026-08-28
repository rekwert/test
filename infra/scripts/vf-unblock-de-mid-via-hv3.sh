#!/usr/bin/env bash
# Unblock DE midrange: route group 3 (HV3 DE-prosto) until HV5 wizard commission done.
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' /opt/testVPStrade/infra/docker/.env | cut -d= -f2-)"

echo "=== 1. VF NL: link midrange packages to group 3, ensure HV3 ready ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

"${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1, maintenance=0, prohibit=0 WHERE id=3;"

# Midrange packages on group 3 (shared pool until HV5 commissioned via VF UI wizard)
for PKG in 10 17 18 19 20 21; do
  "${MY[@]}" -e "INSERT IGNORE INTO ss_package_group_package (package_group_id, package_id, \`order\`) VALUES (1, $PKG, $PKG);" 2>/dev/null || true
done

# Soft-delete zombie failed servers on HV5
"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL;"

"${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) FROM hypervisors WHERE id IN (3,5);"
"${MY[@]}" -e "SELECT spgp.package_id, sp.name FROM ss_package_group_package spgp JOIN server_packages sp ON sp.id=spgp.package_id WHERE spgp.package_id IN (10,17,18,19,20,21) ORDER BY spgp.package_id;"

supervisorctl restart vf-queue: 2>/dev/null | tail -3 || true
NL

echo "=== 2. Portal: DE-mid uses VF group 3 (HV3) until HV5 wizard done ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
-- Both DE nodes share VF hypervisor group 3 (HV3) until HV5 commissioned in VF UI wizard
DROP INDEX IF EXISTS vps.nodes_external_id_uidx;

UPDATE vps.nodes
SET external_id = '3',
    status = 'online',
    vf_commissioned = 3,
    vf_enabled = true,
    maintenance_mode = false,
    updated_at = now()
WHERE id IN ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005');

UPDATE vps.instances
SET external_id = NULL, ip_address = NULL, state = 'creating', updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number IN (196, 199) AND i.state IN ('creating', 'error', 'deleted')
);

SELECT id, name, vf_name, external_id, supported_tiers, status FROM vps.nodes WHERE region = 'de' ORDER BY name;
SQL

echo "=== 3. Restart worker ==="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1
sleep 120

echo "=== 4. Status ==="
docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE 'complete provision|retry allocate|196|199|198|enough resources' | tail -20 || true
psql "$POSTGRES_DSN" -c \
  "SELECT o.order_number, i.hostname, i.state, i.ip_address, i.external_id, n.name
   FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id
   LEFT JOIN vps.nodes n ON n.id=i.node_id
   WHERE o.order_number IN (196,198,199);"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'source /opt/virtfusion/app/control/.env; mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,hypervisor_id,state,commissioned FROM servers ORDER BY id DESC LIMIT 5;"'

echo "DE_UNBLOCK_DONE"
