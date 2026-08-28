#!/bin/bash
echo "=== INTERFACES ==="
cat /etc/network/interfaces 2>/dev/null || cat /etc/netplan/*.yaml 2>/dev/null
echo "=== VF SERVICES ==="
systemctl list-units 'vf-*' --no-pager
echo "=== MARIADB DBS ==="
mysql -e "SHOW DATABASES LIKE 'vf%';" 2>/dev/null || echo no-mysql
echo "=== INSTALL LOG if any ==="
ls -la /root/virtfusion* /var/log/virtfusion* 2>/dev/null | head -10
