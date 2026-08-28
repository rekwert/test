#!/bin/bash
echo "=== SERVICES ==="
systemctl is-active vf-nginx vf-php8-fpm vf-control-wss 2>/dev/null
supervisorctl status 2>/dev/null | head -5

echo ""
echo "=== CURL LOCAL ==="
curl -sk -o /dev/null -w "GET /admin -> HTTP:%{http_code} redirect:%{redirect_url}\n" "https://127.0.0.1/admin"
curl -sk -o /dev/null -w "GET / -> HTTP:%{http_code}\n" "https://127.0.0.1/"
curl -sk -o /dev/null -w "GET /api/v1/compute/hypervisors/1 -> HTTP:%{http_code}\n" -H "Authorization: Bearer test" "https://127.0.0.1/api/v1/compute/hypervisors/1"

echo ""
echo "=== NGINX CONFIG admin ==="
grep -r "admin\|location" /opt/virtfusion/nginx/conf/conf.d/ 2>/dev/null | head -40

echo ""
echo "=== NGINX ERROR LOG tail ==="
tail -20 /opt/virtfusion/nginx/logs/error.log 2>/dev/null

echo ""
echo "=== ACCESS LOG admin ==="
grep " /admin" /opt/virtfusion/nginx/logs/access.log 2>/dev/null | tail -10

echo ""
echo "=== PHP SOCKETS ==="
ls -la /opt/virtfusion/php8/socket/

echo ""
echo "=== CONTROL APP ==="
ls -la /opt/virtfusion/app/control/public/index.php 2>/dev/null
