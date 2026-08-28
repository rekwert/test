#!/bin/bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env

echo "=== INSTANCE gm4kgz / vf 669 ==="
psql "$POSTGRES_DSN" -x -c \
  "SELECT id, hostname, state, external_id, ip_address::text, region, node_id, plan_id, created_at, updated_at
   FROM vps.instances WHERE hostname ILIKE '%gm4kgz%' OR external_id='669';"

echo ""
echo "=== DE NODES ==="
psql "$POSTGRES_DSN" -c \
  "SELECT id, name, region, tiers, vf_hypervisor_id, portal_status, enabled FROM vps.nodes WHERE region='de';"

echo ""
echo "=== VF server 669 summary ==="
curl -sk "${VIRTFUSION_API_URL}/servers/669" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);s=d["data"];print("state",s["state"],"commission",s["commissionStatus"],"hv",s["hypervisorId"],"built",s["built"],"ip",s["network"]["interfaces"][0]["ipv4"] if s.get("network",{}).get("interfaces") else None)'

echo ""
echo "=== VF HV5 ==="
curl -sk "${VIRTFUSION_API_URL}/compute/hypervisors/5" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);h=d.get("data",d);print(json.dumps({k:h.get(k) for k in ("id","name","enabled","commissioned","maintenance","ip")}, indent=2))'

echo ""
echo "=== WORKER logs gm4kgz ==="
docker logs docker-vps-worker-1 2>&1 | grep -i gm4kgz | tail -50 || true
