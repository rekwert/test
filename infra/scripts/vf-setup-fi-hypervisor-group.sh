#!/bin/bash
# Create VirtFusion hypervisor group for FI so portal node external_id=2 maps to a valid VF group.
# NL stays on group 1 — do not modify NL-node1.
set -euo pipefail
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1"; }

FI_HV_ID=2
FI_GROUP_ID=2

echo "=== before ==="
myG "SELECT id,name FROM hypervisor_groups ORDER BY id;"
myG "SELECT id,name,hypervisor_group_id,commissioned FROM hypervisors ORDER BY id;"

GRP=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e \
  "SELECT COUNT(*) FROM hypervisor_groups WHERE id=$FI_GROUP_ID;")
if [[ "$GRP" == "0" ]]; then
  myG "INSERT INTO hypervisor_groups
    (id,name,description,\`default\`,distribution_type,label,type,icon,visible,visible_label,enabled,created_at,updated_at)
    VALUES ($FI_GROUP_ID,'FI','Finland HEL',0,5,NULL,NULL,NULL,0,1,1,NOW(),NOW());"
  echo "created hypervisor group $FI_GROUP_ID"
else
  myG "UPDATE hypervisor_groups SET name='FI', enabled=1 WHERE id=$FI_GROUP_ID;"
  echo "group $FI_GROUP_ID exists"
fi

myG "UPDATE hypervisors SET hypervisor_group_id=$FI_GROUP_ID WHERE id=$FI_HV_ID;"

echo "=== link packages to FI group ==="
for PKG in $(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e \
  "SELECT id FROM server_packages WHERE enabled=1;"); do
  myG "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
    VALUES ($FI_GROUP_ID,$PKG);" 2>/dev/null || true
done

echo "=== self-service group links (best effort) ==="
if mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "DESCRIBE ss_group_hv_group;" &>/dev/null; then
  CNT=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e \
    "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=$FI_GROUP_ID;")
  if [[ "$CNT" == "0" ]]; then
    myG "INSERT INTO ss_group_hv_group (group_id,hypervisor_group_id,name,label,\`order\`)
      VALUES (1,$FI_GROUP_ID,'FI','FI',2);" 2>/dev/null || true
  fi
fi
if mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "DESCRIBE ss_grp_hv_grp_pkg_grp;" &>/dev/null; then
  CNT=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e \
    "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=$FI_GROUP_ID;")
  if [[ "$CNT" == "0" ]]; then
    myG "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id,hypervisor_group_id,group_id,\`order\`)
      VALUES (1,$FI_GROUP_ID,1,2);" 2>/dev/null || true
  fi
fi

echo "=== verify API create with hypervisorId=$FI_GROUP_ID ==="
cd /opt/testVPStrade/infra/docker 2>/dev/null || true
TOKEN=$(grep '^VIRTFUSION_API_KEY=' /opt/testVPStrade/infra/docker/.env | cut -d= -f2-)
BASE=$(grep '^VIRTFUSION_API_URL=' /opt/testVPStrade/infra/docker/.env | cut -d= -f2- | sed 's|/$||')
code=$(curl -sk -o /tmp/vf-fi-grp-test.json -w '%{http_code}' \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -X POST -d "{\"packageId\":1,\"userId\":1,\"hypervisorId\":$FI_GROUP_ID}" \
  "$BASE/servers")
echo "create test: HTTP $code $(head -c 120 /tmp/vf-fi-grp-test.json)"
if [[ "$code" == "201" || "$code" == "200" ]]; then
  SID=$(python3 -c 'import json;print(json.load(open("/tmp/vf-fi-grp-test.json"))["data"]["id"])')
  curl -sk -o /dev/null -X DELETE -H "Authorization: Bearer $TOKEN" "$BASE/servers/$SID?delay=0"
  echo "cleaned probe server $SID"
fi

echo "=== after ==="
myG "SELECT id,name FROM hypervisor_groups ORDER BY id;"
myG "SELECT id,name,hypervisor_group_id,commissioned FROM hypervisors ORDER BY id;"
echo VF_FI_GROUP_DONE
