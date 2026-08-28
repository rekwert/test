#!/bin/bash
echo "=== NGINX ERROR ==="
tail -20 /opt/virtfusion/nginx/logs/error.log 2>/dev/null
echo "=== RESTART NGINX ==="
systemctl restart vf-nginx
sleep 2
systemctl status vf-nginx --no-pager | head -12
ss -tlnp | grep -E ':443|:80 ' || true
curl -sk -o /dev/null -w "443_login:%{http_code}\n" https://127.0.0.1/login
