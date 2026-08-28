#!/bin/bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"

echo "=== VF server 669 now ==="
curl -sk "${VIRTFUSION_API_URL}/servers/669" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);s=d["data"];print("state",s["state"],"commission",s["commissionStatus"],"tasks",s.get("tasks"))'

echo ""
echo "=== NL mysql servers 669 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'REMOTE'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "DESCRIBE servers;" | head -30
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM servers WHERE id=669\G" | head -50
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM server_build_log WHERE server_id=669 ORDER BY id DESC LIMIT 5\G" 2>/dev/null || true
REMOTE

echo ""
echo "=== Instance portal ==="
psql "$POSTGRES_DSN" -c "SELECT id,hostname,state,external_id,ip_address::text FROM vps.instances WHERE external_id='669';"

echo ""
echo "=== Worker last 2m ==="
docker logs docker-vps-worker-1 --since 3m 2>&1 | tail -40
