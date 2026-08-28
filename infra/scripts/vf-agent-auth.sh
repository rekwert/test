#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
TOKEN=$(python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])")

echo "=== AGENT WITH AUTH TOKEN ==="
for path in /health /status /hypervisor/resources /hypervisor/status /local/hypervisor/resources; do
  code=$(curl -sk -o /tmp/vf-agent-out -w '%{http_code}' -H "Authorization: Bearer $TOKEN" "https://127.0.0.1:8892$path")
  echo "$path HTTP:$code $(head -c 200 /tmp/vf-agent-out)"
done

echo ""
echo "=== GLOBAL API TOKEN DB ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,name,enabled,LEFT(token,40) tok,permissions,access FROM global_api_tokens\G" 2>/dev/null

echo "=== TEST LOCAL API WITH DB TOKEN ==="
DBTOK=$(mysql -N -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT token FROM global_api_tokens WHERE id=1;" 2>/dev/null)
curl -sk -X POST "https://127.0.0.1/api/v1/servers" \
  -H "Authorization: Bearer $DBTOK" \
  -H "Content-Type: application/json" \
  -d '{"packageId":1,"userId":1,"hypervisorId":1}' | head -c 300
echo ""

echo "=== GREP Not valid in control app (strings) ==="
grep -r 'Not valid' /opt/virtfusion/app/control/app/ 2>/dev/null | head -5
strings /opt/virtfusion/app/control/app/Http/Controllers/Api/V1/ServerController.php 2>/dev/null | grep -i 'valid\|EC' | head -10

echo "=== PACKAGE TYPE ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,name,enabled,type FROM server_packages;" 2>/dev/null
