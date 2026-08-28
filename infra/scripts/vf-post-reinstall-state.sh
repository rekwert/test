#!/bin/bash
source /opt/virtfusion/app/control/.env 2>/dev/null || true
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== start libvirtd ==="
systemctl enable libvirtd 2>/dev/null || true
systemctl start libvirtd 2>/dev/null || true
systemctl is-active libvirtd

echo "=== hypervisors ==="
myG "SELECT id,name,enabled,commissioned,ip FROM hypervisors;"

echo "=== packages ==="
myG "SELECT id,name,enabled FROM server_packages;"

echo "=== api tokens ==="
myG "SELECT id,name,enabled,access,LEFT(token_1,20) tok FROM global_api_tokens;"

echo "=== license ==="
myG "SELECT \`key\`, LEFT(value,60) FROM configuration WHERE \`key\` LIKE 'license%';"

echo "=== users ==="
myG "SELECT id,name,email,enabled FROM users;"

echo "=== ipv4 ==="
myG "SELECT COUNT(*) AS ipv4_count FROM ipv4;" 2>/dev/null || echo 0
