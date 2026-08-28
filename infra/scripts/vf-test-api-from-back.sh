#!/bin/bash
set -euo pipefail
set -a
source /opt/testVPStrade/infra/docker/.env
set +a

echo "=== POST /servers ==="
curl -sk -w "\nHTTP:%{http_code}\n" -X POST "${VIRTFUSION_API_URL}/servers" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"packageId":1,"userId":1,"hypervisorId":1}'

echo ""
echo "=== GET /compute/hypervisors/1 ==="
curl -sk "${VIRTFUSION_API_URL}/compute/hypervisors/1" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | python3 -m json.tool 2>/dev/null | head -80
