#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env
BASE="${VIRTFUSION_API_URL%/}/"
AUTH="Authorization: Bearer ${VIRTFUSION_API_KEY}"

echo "=== connect ==="
curl -sk -H "$AUTH" "$BASE/connect"
echo

echo "=== package templates ==="
curl -sk -H "$AUTH" "$BASE/media/templates/fromServerPackageSpec/1" | head -c 3000
echo

echo "=== hypervisors ==="
curl -sk -H "$AUTH" "$BASE/compute/hypervisors?results=5" | head -c 1200
echo

echo "=== nodes db ==="
psql "$POSTGRES_DSN" -At -c "SELECT id,name,region,status,external_id FROM vps.nodes;"
