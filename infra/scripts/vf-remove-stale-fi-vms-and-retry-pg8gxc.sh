#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a; source .env; source .env.probe; set +a

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP_DIR="/root/ops-backups/fi-stale-$STAMP"
install -d -m 700 "$BACKUP_DIR"

echo "=== Back up portal and VirtFusion API records ==="
psql "$POSTGRES_DSN" -c "COPY (
  SELECT * FROM vps.instances
  WHERE node_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002'
    AND external_id IN ('35','37','46','47','48')
) TO STDOUT WITH CSV HEADER" >"$BACKUP_DIR/portal_instances.csv"
for id in 35 37 46 47 48; do
  curl -fsS -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$id" >"$BACKUP_DIR/vf-server-$id.json" || true
done
chmod -R go-rwx "$BACKUP_DIR"

echo "=== Request immediate deletion through VirtFusion API ==="
for id in 35 37 46 47 48; do
  body="/tmp/vf-delete-$id.json"
  code=$(curl -sS -o "$body" -w '%{http_code}' \
    -X DELETE -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$id?delay=0")
  echo "server=$id http=$code body=$(tr '\n' ' ' <"$body")"
done
sleep 12

echo "=== Retire any records left behind by missing physical domains ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE servers SET state='deleted',deleted_at=COALESCE(deleted_at,NOW()),updated_at=NOW() WHERE id IN (35,37,46,47,48) AND deleted_at IS NULL;"
"${MY[@]}" -e "UPDATE ipv4 SET server_id=NULL WHERE server_id IN (35,37,46,47,48);"
"${MY[@]}" -e "SELECT id,name,state,deleted_at FROM servers WHERE id IN (35,37,46,47,48) ORDER BY id;"
"${MY[@]}" -e "SELECT id,INET_NTOA(address) ip,block_id,server_id FROM ipv4 WHERE block_id IN (2,3) ORDER BY address;"
NL

echo "=== Hide linked stale portal instances and stop their billing lifecycle ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET state='deleted',
    billing_status='cancelled',
    external_id=NULL,
    ip_address=NULL,
    next_billing_at=NULL,
    provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      || jsonb_build_object('cleanup_reason','fi_host_rebuilt_no_disks','cleaned_at',now()),
    updated_at=now()
WHERE node_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002'
  AND external_id IN ('35','37','46','47','48');

UPDATE vps.instances
SET state='deleted',
    billing_status='cancelled',
    external_id=NULL,
    ip_address=NULL,
    next_billing_at=NULL,
    provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      || jsonb_build_object('cleanup_reason','fi_host_rebuilt_vm_missing','cleaned_at',now()),
    updated_at=now()
WHERE id='2f22d8f2-72b5-438c-a2a7-a6342b42fdd0'
  AND node_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002'
  AND state='reinstalling';

SELECT hostname,state,billing_status,external_id,ip_address
FROM vps.instances
WHERE node_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002'
ORDER BY created_at;
SQL

echo "=== Reset current FI order for a clean worker retry ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET state='creating',
    external_id=NULL,
    ip_address=NULL,
    provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      - 'provision_error' - 'provision_error_at' - 'provision_allocate_claim'
      - 'worker_poll_claimed_at' - 'worker_poll_claimed_by',
    worker_poll_claimed_at=NULL,
    worker_poll_claimed_by=NULL,
    updated_at=now()
WHERE hostname='vps-pg8gxc';
SQL

docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null ||
  docker restart docker-vps-worker-1
echo "backup=$BACKUP_DIR"
