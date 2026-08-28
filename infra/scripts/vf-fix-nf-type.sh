#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== BEFORE nf_type ==="
myG "SELECT id,nf_type,enabled,commissioned FROM hypervisors WHERE id=1;"

echo "=== SET nf_type=1 (bridged) ==="
myG "UPDATE hypervisors SET nf_type=1, updated_at=NOW() WHERE id=1;"
myG "SELECT id,nf_type FROM hypervisors WHERE id=1;"

echo "=== CREATE END USER id=2 ==="
CNT=$(mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e "SELECT COUNT(*) FROM users WHERE id=2;" 2>/dev/null)
if [ "$CNT" = "0" ]; then
  myG "INSERT INTO users (id,name,email,password,enabled,admin,timezone,tfa,self_service,self_service_bal,api_rpm,locale,email_verified_at,created_at,updated_at)
SELECT 2,'VPS Bot','vps-bot@cloud-hustle.local',password,1,0,timezone,tfa,1,0,api_rpm,locale,NOW(),NOW(),NOW() FROM users WHERE id=1;"
fi
myG "SELECT id,name,admin,self_service FROM users;"

supervisorctl restart vf-queue-hv:00 vf-queue-hv:01 2>/dev/null | tail -3
