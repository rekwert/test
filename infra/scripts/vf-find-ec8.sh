#!/bin/bash
echo "=== SEARCH EC in language files ==="
grep -r '\[EC' /opt/virtfusion/app/control/language/ 2>/dev/null | head -30
grep -r 'EC.8\|EC: 8\|EC 8' /opt/virtfusion/app/control/ 2>/dev/null | grep -v vendor | grep -v '.git' | head -20

echo "=== SEARCH in storage cache ==="
grep -r 'EC 8\|EC: 8' /opt/virtfusion/app/control/storage/ 2>/dev/null | head -10

echo "=== ALL SERVERS ANY STATE ==="
source /opt/virtfusion/app/control/.env
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,state,commissioned,hypervisor_id,owner_id FROM servers;" 2>/dev/null

echo "=== DISK FILES leftover ==="
find /home/vf-data/disk -maxdepth 2 -type f 2>/dev/null | head -20
ls -la /home/vf-data/disk/ 2>/dev/null | head -20

echo "=== LIBVIRT DOMAINS ==="
virsh list --all 2>/dev/null
