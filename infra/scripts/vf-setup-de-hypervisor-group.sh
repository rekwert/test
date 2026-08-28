#!/bin/bash
# Create VirtFusion hypervisor group 3 for DE; link DE-prosto hypervisor.
set -euo pipefail
source /opt/virtfusion/app/control/.env

myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1"; }
myN() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "$1"; }

DE_HV_ID=3
DE_GROUP_ID=3
DE_IP=185.84.224.84

echo "=== before ==="
myG "SELECT id,name,ip,hypervisor_group_id,commissioned FROM hypervisors ORDER BY id;"
myG "SELECT id,name FROM hypervisor_groups ORDER BY id;"

GRP=$(myN "SELECT COUNT(*) FROM hypervisor_groups WHERE id=$DE_GROUP_ID;")
if [[ "$GRP" == "0" ]]; then
  myG "INSERT INTO hypervisor_groups
    (id,name,description,\`default\`,distribution_type,label,type,icon,visible,visible_label,enabled,created_at,updated_at)
    VALUES ($DE_GROUP_ID,'DE','Germany Frankfurt',0,5,NULL,NULL,NULL,0,1,1,NOW(),NOW());"
  echo "created hypervisor group $DE_GROUP_ID"
else
  myG "UPDATE hypervisor_groups SET name='DE', enabled=1 WHERE id=$DE_GROUP_ID;"
fi

myG "UPDATE hypervisors SET hypervisor_group_id=$DE_GROUP_ID, name='DE-prosto' WHERE ip='$DE_IP';"

echo "=== link packages to DE group ==="
for PKG in $(myN "SELECT id FROM server_packages WHERE enabled=1;"); do
  myG "INSERT IGNORE INTO hypervisor_group_server_package (hypervisor_group_id, server_package_id)
    VALUES ($DE_GROUP_ID,$PKG);" 2>/dev/null || true
done

ST=$(myN "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=$DE_HV_ID;")
if [[ "$ST" == "0" ]]; then
  myG "INSERT INTO hypervisor_storage
    (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at)
    VALUES ($DE_HV_ID,'Local disk','/home/vf-data/disk','mountpoint',2000,0,1,1,'[]',NOW(),NOW());"
fi

echo "=== after ==="
myG "SELECT id,name,ip,hypervisor_group_id,commissioned FROM hypervisors ORDER BY id;"
myG "SELECT id,name FROM hypervisor_groups ORDER BY id;"
echo VF_DE_GROUP_DONE
