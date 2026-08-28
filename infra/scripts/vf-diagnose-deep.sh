#!/bin/bash
echo "=== HYPERVISOR APP LOG (tail) ==="
tail -80 /opt/virtfusion/app/hypervisor/storage/logs/app-2026-07-12.log 2>/dev/null

echo "=== HYPERVISOR EVENT LOG (tail) ==="
tail -80 /opt/virtfusion/app/hypervisor/storage/logs/event-2026-07-12.log 2>/dev/null

echo "=== CONTROL PHP LOG (filtered) ==="
tail -200 /opt/virtfusion/php8/var/log/php.log 2>/dev/null | grep -iE 'EC 8|Not valid|error|exception|alloc|hypervisor|accept' | tail -40

echo "=== FIND EC8 IN VF LOGS ONLY ==="
grep -r 'EC 8\|\[EC 8\]' /opt/virtfusion/app/hypervisor/storage/logs/ /opt/virtfusion/php8/var/log/ /opt/virtfusion/app/control/storage/ 2>/dev/null | tail -30

echo "=== CONTROL STORAGE ==="
ls -la /opt/virtfusion/app/control/storage/logs/ 2>/dev/null
ls -la /opt/virtfusion/app/control/storage/ 2>/dev/null | head -20

echo "=== DB CREDS (masked) ==="
grep -E '^DB_' /opt/virtfusion/app/control/.env 2>/dev/null | sed 's/PASSWORD=.*/PASSWORD=***/'

echo "=== MYSQL QUERIES ==="
set -a
# shellcheck disable=SC1091
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
DBH="${DB_HOST:-127.0.0.1}"
DBU="${DB_USERNAME:-root}"
DBP="${DB_PASSWORD:-}"
DBN="${DB_DATABASE:-virtfusion}"
my() { mysql -h"$DBH" -u"$DBU" -p"$DBP" "$DBN" -e "$1" 2>/dev/null; }

my "SHOW TABLES LIKE '%hypervisor%';"
my "SHOW TABLES LIKE '%resource%';"
my "SHOW TABLES LIKE '%package%';"
my "SELECT id,name,enabled,commissioned,maintenance,accept FROM hypervisors WHERE id=1;"
my "DESCRIBE packages;" | head -40
my "SELECT * FROM packages WHERE id=1\G"
my "SELECT * FROM resource_allocation_logs ORDER BY id DESC LIMIT 5\G"
my "SELECT * FROM licenses LIMIT 3\G"

echo "=== EC 8 IN PHP SOURCE ==="
grep -r 'EC 8' /opt/virtfusion/app/control/app/ 2>/dev/null | head -15

echo "=== API ACCEPT (needs token from panel - skip if none) ==="
curl -sk https://127.0.0.1/api/v1/compute/hypervisors/groups/1/resources 2>/dev/null | head -c 500
