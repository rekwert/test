#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
source .env

echo "=== VirtFusion API records still assigned to rebuilt FI ==="
for id in 35 37 46 47 48; do
  curl -fsS -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$id" |
    python3 -c 'import json,sys
d=(json.load(sys.stdin).get("data") or {})
n=d.get("network") or {}
ips=[x.get("address") for i in n.get("interfaces",[]) for x in (i.get("ipv4") or []) if isinstance(x,dict)]
print({k:d.get(k) for k in ("id","name","state","hypervisorId","packageId","userId","createdAt")}, "ips=",ips)'
done

echo "=== Portal instances mapped to FI ==="
psql "$POSTGRES_DSN" -c "SELECT i.id,i.hostname,i.state,i.external_id,i.ip_address,o.order_number,i.provider_meta FROM vps.instances i LEFT JOIN vps.orders o ON o.id=i.order_id WHERE i.node_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002' AND i.state NOT IN ('deleted') ORDER BY i.created_at;"

echo "=== FI IP allocations ==="
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT id,INET_NTOA(address) ip,block_id,server_id FROM ipv4 WHERE block_id IN (2,3) ORDER BY address;"
echo "=== Backup-related tables ==="
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SHOW TABLES LIKE '%backup%';"
echo "=== Legacy server backups for FI records ==="
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT * FROM server_backups WHERE server_id IN (35,37,46,47,48);"
echo "=== Backup manager entries mentioning FI records ==="
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT * FROM backup_mgr_backups WHERE server_id IN (35,37,46,47,48);"
NL
