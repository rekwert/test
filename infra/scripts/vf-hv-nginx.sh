#!/bin/bash
echo "=== HV NGINX CONF ==="
cat /opt/virtfusion/nginx/conf/conf.d/hv.conf

echo ""
echo "=== LOCAL INDEX ROUTES ==="
ls -la /opt/virtfusion/app/hypervisor/local/ 2>/dev/null
ls -la /opt/virtfusion/app/hypervisor/public/ 2>/dev/null | head

echo ""
echo "=== TRY LOCAL ENDPOINTS ==="
# Control decrypts DB token and sends special headers
DBTOK=$(mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT token FROM hypervisors WHERE id=1;" 2>/dev/null)
for path in /local/hypervisor/resources /local/hypervisor/status /local/health; do
  code=$(curl -sk -o /tmp/out -w '%{http_code}' -H "Authorization: Bearer $DBTOK" "https://127.0.0.1:8892$path")
  echo "$path -> $code $(head -c 120 /tmp/out)"
done

echo ""
echo "=== PHP SOCKETS ==="
ls -la /opt/virtfusion/php8/socket/

echo ""
echo "=== CHECK LICENSE FROM PANEL ENDPOINT ==="
curl -sk -b /tmp/vf-cookie "https://127.0.0.1/admin/configuration/license.json" 2>/dev/null | head -c 500
