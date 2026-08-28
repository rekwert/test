#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a

echo "=== GLOBAL API TOKENS SCHEMA ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "DESCRIBE global_api_tokens;" 2>/dev/null
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM global_api_tokens\G" 2>/dev/null

echo "=== HYPERVISOR AGENT LOG TAIL ==="
tail -50 /opt/virtfusion/app/hypervisor/storage/logs/app-$(date +%Y-%m-%d).log 2>/dev/null || ls -la /opt/virtfusion/app/hypervisor/storage/logs/ | tail -5

echo "=== NGINX HYPERVISOR CONFIG ==="
grep -r '8892\|hypervisor' /opt/virtfusion/nginx/ 2>/dev/null | head -20

echo "=== TRY AUTH HEADERS ==="
TOK=$(python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])")
HASH=$(python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['hash'])")
HID=$(python3 -c "import json; print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['id'])")

for hdr in \
  "Authorization: Bearer $TOK" \
  "X-Auth-Token: $TOK" \
  "X-Hypervisor-Token: $TOK" \
  "X-Virtfusion-Token: $TOK"; do
  code=$(curl -sk -o /tmp/out -w '%{http_code}' -H "$hdr" "https://127.0.0.1:8892/health")
  echo "$hdr -> $code $(head -c 80 /tmp/out)"
done

echo "=== HYPERVISOR ROUTES ==="
grep -r "health\|unauthenticated" /opt/virtfusion/app/hypervisor/routes/ 2>/dev/null | head -10

echo "=== COMMISSION JOBS PENDING ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM hypervisor_jobs WHERE hypervisor_id=1 ORDER BY id DESC LIMIT 5;" 2>/dev/null
