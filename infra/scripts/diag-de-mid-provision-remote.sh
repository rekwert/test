#!/bin/bash
set -euo pipefail

echo "=== POSTGRES by external_id ==="
docker exec docker-postgres-1 psql -U vps -d vps_platform -x -c \
  "SELECT id, hostname, state, external_id, ip_address::text, node_id, provider_meta, created_at, updated_at
   FROM vps.instances WHERE external_id='669';"

echo ""
echo "=== POSTGRES recent DE creating/failed ==="
docker exec docker-postgres-1 psql -U vps -d vps_platform -c \
  "SELECT id, hostname, state, external_id, ip_address::text, region, created_at
   FROM vps.instances WHERE region='de' AND created_at > now() - interval '6 hours'
   ORDER BY created_at DESC LIMIT 10;"

echo ""
echo "=== VF hypervisor 5 ==="
set -a
source /opt/testVPStrade/infra/docker/.env
set +a
curl -sk "${VIRTFUSION_API_URL}/compute/hypervisors/5" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);h=d.get("data",d);print("id",h.get("id"),"name",h.get("name"),"enabled",h.get("enabled"),"commissioned",h.get("commissioned"),"maintenance",h.get("maintenance"));r=h.get("resources") or {};print("servers",r.get("servers"));print("memory",r.get("memory"));print("storage",r.get("localStorage"))'

echo ""
echo "=== NL MYSQL ==="
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=12 root@66.248.206.14 'bash -s' <<'EOS'
set -a
source /opt/virtfusion/app/control/.env
set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" <<'SQL'
SELECT id,name,enabled,commissioned,maintenance FROM hypervisors ORDER BY id;
SELECT id,hypervisor_id,commission_status,state,built,build_failed,hostname FROM servers WHERE id=669\G
SHOW TABLES LIKE '%task%';
SQL
EOS

echo ""
echo "=== DE-mid logs via NL ==="
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=12 root@66.248.206.14 'bash -s' <<'EOS'
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=12 root@66.151.40.165 'bash -s' <<'INNER'
LOG=/opt/virtfusion/app/hypervisor/storage/logs/app-$(date +%Y-%m-%d).log
echo "=== services ==="
systemctl is-active vf-nginx vf-php8-fpm supervisor libvirtd 2>/dev/null || true
supervisorctl status 2>/dev/null | head -8 || true
echo "=== virsh ==="
virsh list --all 2>/dev/null | head -12 || true
echo "=== log grep 669 ==="
grep -i 669 "$LOG" 2>/dev/null | tail -30 || echo "no 669 lines"
echo "=== log errors tail ==="
grep -iE 'error|fail|exception|commission|storage|libvirt|build' "$LOG" 2>/dev/null | tail -40 || tail -20 "$LOG" 2>/dev/null
INNER
EOS

echo ""
echo "=== VF build log API if any ==="
curl -sk "${VIRTFUSION_API_URL}/servers/669/build" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | head -c 2000; echo

echo ""
echo "=== worker logs ==="
docker logs docker-vps-worker-1 2>&1 | grep -E '6677945d|gm4kgz|/669|external_id.*669' | tail -40 || true
