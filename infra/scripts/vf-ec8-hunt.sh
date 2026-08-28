#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a

echo "=== guest.json EC lines ==="
python3 - <<'PY'
import json,re
for path in ['/opt/virtfusion/app/control/language/en/guest.json',
             '/opt/virtfusion/app/control/language/en/user_self_service_create.json']:
    try:
        d=json.load(open(path))
        s=json.dumps(d)
        for m in re.finditer(r'.{0,40}EC.{0,40}', s):
            print(path.split('/')[-1], m.group())
    except Exception as e:
        print(path, e)
PY

echo "=== grep EC in all json ==="
grep -r '"EC' /opt/virtfusion/app/control/language/en/ 2>/dev/null | head -40
grep -r 'Not valid' /opt/virtfusion/app/control/language/en/ 2>/dev/null | head -20

echo "=== find vfcli ==="
find /opt/virtfusion -name 'vfcli*' 2>/dev/null | head -10

echo "=== ip tables ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SHOW TABLES LIKE 'ip%';" 2>/dev/null

echo "=== hypervisor jobs/tasks ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,hypervisor_id,type,status,created_at FROM hypervisor_tasks ORDER BY id DESC LIMIT 10;" 2>/dev/null
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT id,hypervisor_id,job,status,created_at FROM hypervisor_jobs ORDER BY id DESC LIMIT 10;" 2>/dev/null

echo "=== call hypervisor local metrics ==="
TOKEN=$(mysql -N -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT token FROM hypervisors WHERE id=1;" 2>/dev/null)
echo token_len=${#TOKEN}
curl -sk -H "Authorization: Bearer $TOKEN" "https://127.0.0.1:8892/hypervisor/resources" 2>/dev/null | head -c 1500
