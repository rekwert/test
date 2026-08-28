#!/bin/bash
set -euo pipefail

HOSTNAME_FILTER="${1:-gm4kgz}"
VF_SERVER_ID="${2:-669}"

echo "=== INSTANCE (postgres) ==="
docker exec docker-postgres-1 psql -U vps -d vps_platform -x -c \
  "SELECT id, hostname, status, external_id, ip, node_id, plan_id, os_template_id, created_at, updated_at
   FROM vps.instances WHERE hostname LIKE '%${HOSTNAME_FILTER}%' OR external_id = '${VF_SERVER_ID}'
   ORDER BY created_at DESC LIMIT 3;" 2>/dev/null || true

echo ""
echo "=== NODE for instance ==="
docker exec docker-postgres-1 psql -U vps -d vps_platform -t -A -c \
  "SELECT n.id, n.name, n.region, n.tiers, n.vf_hypervisor_id, n.status, n.portal_status
   FROM vps.nodes n
   JOIN vps.instances i ON i.node_id = n.id
   WHERE i.hostname LIKE '%${HOSTNAME_FILTER}%' OR i.external_id = '${VF_SERVER_ID}'
   LIMIT 1;" 2>/dev/null || true

echo ""
echo "=== VF SERVER ${VF_SERVER_ID} ==="
set -a
source /opt/testVPStrade/infra/docker/.env
set +a
curl -sk "${VIRTFUSION_API_URL}/servers/${VF_SERVER_ID}" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  -o "/tmp/vf-server-${VF_SERVER_ID}.json"
python3 - <<PY
import json
p="/tmp/vf-server-${VF_SERVER_ID}.json"
with open(p) as f:
    d=json.load(f)
s=d.get("data",d)
print("id:", s.get("id"))
print("state:", s.get("state"))
print("commissionStatus:", s.get("commissionStatus"))
print("built:", s.get("built"))
print("buildFailed:", s.get("buildFailed"))
hv=s.get("hypervisor") or {}
print("hypervisor:", hv.get("id"), hv.get("name"), hv.get("ip"))
print("hostname:", s.get("hostname"))
ifaces=(s.get("network") or {}).get("interfaces") or []
if ifaces:
    print("ipv4:", ifaces[0].get("ipv4"))
print("os:", s.get("os"))
print("tasks:", json.dumps(s.get("tasks"), default=str)[:300])
PY

echo ""
echo "=== WORKER LOGS (instance / vf ${VF_SERVER_ID}) ==="
docker logs docker-vps-worker-1 2>&1 | grep -iE "${HOSTNAME_FILTER}|${VF_SERVER_ID}|not commissioned|resetPassword|complete provision|retry build" | tail -40 || true

echo ""
echo "=== VF HYPERVISORS (NL panel mysql via ssh) ==="
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=8 root@66.248.206.14 \
  'set -a; source /opt/virtfusion/app/control/.env; set +a; mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,name,enabled,commissioned,maintenance FROM hypervisors ORDER BY id;"' 2>/dev/null || echo "NL mysql via ssh failed"

echo ""
echo "=== VF SERVER ROW (NL mysql) ==="
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=8 root@66.248.206.14 \
  "set -a; source /opt/virtfusion/app/control/.env; set +a; mysql -h\"\$DB_HOST\" -u\"\$DB_USERNAME\" -p\"\$DB_PASSWORD\" \"\$DB_DATABASE\" -e \"SELECT id,hypervisor_id,commission_status,state,built,build_failed,hostname FROM servers WHERE id=${VF_SERVER_ID}\\\\G\"" 2>/dev/null || echo "NL server row failed"
