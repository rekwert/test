#!/bin/bash
set -a
source /opt/testVPStrade/infra/docker/.env
set +a

echo "=== hypervisor group resources ==="
curl -sk "${VIRTFUSION_API_URL}/compute/hypervisors/groups/1/resources" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | python3 -m json.tool 2>/dev/null | head -100

echo ""
echo "=== POST variants ==="
for body in \
  '{"packageId":1,"userId":1,"hypervisorId":1}' \
  '{"packageId":1,"userId":2,"hypervisorId":1}' \
  '{"packageId":1,"userId":1,"hypervisorGroupId":1}' \
  '{"packageId":1,"userId":2,"hypervisorGroupId":1}'; do
  echo "BODY: $body"
  curl -sk -w " HTTP:%{http_code}\n" -X POST "${VIRTFUSION_API_URL}/servers" \
    -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$body"
  echo ""
done

echo "=== GET packages ==="
curl -sk "${VIRTFUSION_API_URL}/servers/packages/1" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | python3 -m json.tool 2>/dev/null | head -60

echo "=== GET users ==="
curl -sk "${VIRTFUSION_API_URL}/users/1" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | python3 -m json.tool 2>/dev/null | head -40
