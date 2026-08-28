#!/bin/bash
set -a
source /opt/testVPStrade/infra/docker/.env
set +a

echo "=== HYPERVISOR RESOURCES ==="
curl -sk "${VIRTFUSION_API_URL}/compute/hypervisors/groups/1/resources" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" | python3 -c "
import json,sys
d=json.load(sys.stdin)['data'][0]
h=d['hypervisor']; r=d['resources']
print('accept=', h.get('accept'))
print('commissioned=', h.get('commissioned'))
print('servers max/free=', r['servers']['max'], r['servers']['free'])
print('ipv4 free=', r['network']['total']['ipv4']['free'])
"

echo ""
echo "=== POST /servers ==="
curl -sk -w "\nHTTP:%{http_code}\n" -X POST "${VIRTFUSION_API_URL}/servers" \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"packageId":1,"userId":1,"hypervisorId":1}'
