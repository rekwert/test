#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
T1=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT token_1 FROM global_api_tokens WHERE id=1;")
T2=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT token_2 FROM global_api_tokens WHERE id=1;")
echo "TOKEN1_LEN ${#T1}"
echo "TOKEN1_PREFIX ${T1:0:12}"
echo "TOKEN2_LEN ${#T2}"
echo "COMBINED_LEN $((${#T1}+${#T2}))"
