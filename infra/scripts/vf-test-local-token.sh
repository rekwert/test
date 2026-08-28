#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
T1=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT token_1 FROM global_api_tokens WHERE id=1;")
T2=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT token_2 FROM global_api_tokens WHERE id=1;")
BODY='{"packageId":1,"userId":1,"hypervisorId":1,"ipv4":1}'
for tok in "${T1}${T2}" "${T1}.${T2}"; do
  echo "try token format len=${#tok}"
  code=$(curl -sk -o /tmp/vfresp.txt -w '%{http_code}' -X POST https://127.0.0.1/api/v1/servers \
    -H "Authorization: Bearer ${tok}" -H 'Content-Type: application/json' -d "$BODY")
  echo "HTTP $code $(head -c 200 /tmp/vfresp.txt)"
done
