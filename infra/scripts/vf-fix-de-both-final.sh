#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env
ROOT=/opt/testVPStrade
POSTGRES_DSN="$(grep '^POSTGRES_DSN=' "$ROOT/infra/docker/.env" | cut -d= -f2-)"

echo "=== 1. Link SS groups for HV group 5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

EXISTS=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")
if [[ "$EXISTS" == "0" ]]; then
  NEXT=$("${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES ($NEXT, 5, 'DE Mid', 'DE Mid', 5);"
  echo "inserted ss_group_hv_group gid=$NEXT hv=5"
fi
GID=$("${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
EXISTS2=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;")
if [[ "$EXISTS2" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 5, $GID, 5);"
  echo "inserted ss_grp_hv_grp_pkg_grp"
fi

"${MY[@]}" -e "SELECT * FROM ss_group_hv_group WHERE hypervisor_group_id=5;"
"${MY[@]}" -e "SELECT * FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;"
"${MY[@]}" -e "SELECT id,name,commissioned,LENGTH(token) tok FROM hypervisors WHERE id IN (3,5);"

echo "=== agent health HV5 ==="
AUTHTOK=$(python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])" 2>/dev/null || \
  ssh -o BatchMode=yes root@66.151.40.165 python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])")
curl -sk -m 10 "https://66.151.40.165:8892/health" -H "Authorization: Bearer $AUTHTOK" -w "\nHTTP=%{http_code}\n"

supervisorctl restart vf-queue-hv: vf-queue-control: 2>/dev/null | tail -3 || true
NL

echo "=== 2. Ensure HV3 commissioned=3 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "mysql -h127.0.0.1 -u\$(grep DB_USERNAME /opt/virtfusion/app/control/.env | cut -d= -f2) -p\$(grep DB_PASSWORD /opt/virtfusion/app/control/.env | cut -d= -f2) \$(grep DB_DATABASE /opt/virtfusion/app/control/.env | cut -d= -f2) -e 'UPDATE hypervisors SET commissioned=3, enabled=1 WHERE id=3;'"

echo "=== 3. Reset stuck mid orders + restart worker ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET external_id = NULL, ip_address = NULL, state = 'creating', updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number IN (199,196) AND i.state IN ('creating', 'error')
);
UPDATE vps.nodes SET vf_commissioned = 3, status = 'online', updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';
SQL

cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo "=== 4. Wait 120s ==="
sleep 120

echo "=== 5. Worker + portal ==="
docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE 'complete provision|retry allocate|enough resources|401|198|199|196|DE-mid|DE-1' | tail -30 || true

psql "$POSTGRES_DSN" -c \
  "SELECT o.order_number, i.hostname, i.state, i.ip_address, i.external_id, n.name
   FROM vps.instances i JOIN vps.orders o ON o.id=i.order_id
   LEFT JOIN vps.nodes n ON n.id=i.node_id
   WHERE o.order_number IN (198,199,196) OR (n.region='de' AND i.state IN ('creating','running') AND i.created_at > now()-interval '24 hours');"

echo "DE_FIX_COMPLETE"
