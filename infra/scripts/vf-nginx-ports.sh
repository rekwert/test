#!/bin/bash
echo "=== PORTS ==="
ss -tlnp | head -20
echo "=== NGINX CONF ==="
ls /opt/virtfusion/nginx/conf/conf.d/
grep -h listen /opt/virtfusion/nginx/conf/conf.d/*.conf 2>/dev/null
echo "=== CURL 443 ==="
curl -sk -o /dev/null -w "443:%{http_code}\n" https://127.0.0.1:443/login 2>/dev/null || echo 443_fail
curl -sk -o /dev/null -w "8892:%{http_code}\n" https://127.0.0.1:8892/ 2>/dev/null || echo 8892_fail
