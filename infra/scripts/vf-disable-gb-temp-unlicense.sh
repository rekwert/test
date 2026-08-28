#!/usr/bin/env bash
# VF license counts all hypervisor rows — disable flag is not enough.
# Backup GB row + links, then remove HV id=4 from VF until license upgraded.
set -euo pipefail
NL_SSH_PASS="${NL_SSH_PASS:-zx_zvJdI9P}"
GB_HV_ID="${GB_HV_ID:-4}"

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 'bash -s' <<REMOTE
set -euo pipefail
source /opt/virtfusion/app/control/.env
BK=/root/gb-hv-backup-\$(date +%Y%m%d).sql
mysql -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" -N -e "SELECT COUNT(*) FROM hypervisors WHERE id=${GB_HV_ID};" | grep -q '^1$' || { echo "GB HV ${GB_HV_ID} missing"; exit 1; }

mysqldump -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" \
  hypervisors hypervisor_networks hypervisor_storage \
  ip_block_hypervisor ip_block_hypervisor_network \
  --where="id=${GB_HV_ID}" 2>/dev/null > "\$BK.partial" || true

# Full row backup via SELECT INTO outfile style
mysql -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" <<SQL > "\$BK"
-- GB hypervisor backup $(date -Iseconds)
SELECT * FROM hypervisors WHERE id=${GB_HV_ID}\G
SELECT * FROM hypervisor_networks WHERE hypervisor_id=${GB_HV_ID}\G
SELECT * FROM hypervisor_storage WHERE hypervisor_id=${GB_HV_ID}\G
SELECT * FROM ip_block_hypervisor WHERE hypervisor_id=${GB_HV_ID};
SELECT * FROM ip_block_hypervisor_network WHERE hypervisor_id=${GB_HV_ID};
SQL

echo "backup: \$BK"

mysql -h"\$DB_HOST" -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" <<SQL
DELETE FROM ip_block_hypervisor_network WHERE hypervisor_id=${GB_HV_ID};
DELETE FROM ip_block_hypervisor WHERE hypervisor_id=${GB_HV_ID};
DELETE FROM hypervisor_storage WHERE hypervisor_id=${GB_HV_ID};
DELETE FROM hypervisor_networks WHERE hypervisor_id=${GB_HV_ID};
DELETE FROM hypervisors WHERE id=${GB_HV_ID};
SELECT COUNT(*) AS hv_count FROM hypervisors;
SELECT id,name,ip,enabled FROM hypervisors ORDER BY id;
SQL
REMOTE

echo "=== worker check in 10s ==="
sleep 10
ssh -o BatchMode=yes root@198.13.189.75 "docker logs docker-vps-worker-1 --since 15s 2>&1 | grep -iE 'license|POST /servers|allocate|build_server|error' | tail -12"
