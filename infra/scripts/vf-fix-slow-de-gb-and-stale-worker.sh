#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a; source .env; source .env.probe; set +a

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP_DIR="/root/ops-backups/de-gb-worker-$STAMP"
install -d -m 700 "$BACKUP_DIR"
psql "$POSTGRES_DSN" -c "COPY (
  SELECT * FROM vps.instances WHERE id IN (
    'd388fad7-96c6-4d28-a679-d09142d02973',
    '6677945d-15e2-46e1-b36b-48dd84e61c0b',
    '7bf455ad-45a2-489a-ae13-bea4260c6844',
    'eda519e1-afdc-47cc-a9e7-502dbbcfc146'
  )
) TO STDOUT WITH CSV HEADER" >"$BACKUP_DIR/instances.csv"

echo "=== Restore DE-prosto scheduler capacity ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE hypervisors SET max_cpu=160,enabled=1,maintenance=0,prohibit=0 WHERE id=3;"
"${MY[@]}" -e "SELECT id,name,max_cpu,max_memory,max_servers,enabled,maintenance,prohibit FROM hypervisors WHERE id=3;"
NL

echo "=== Remove stale VirtFusion shells ==="
for id in 656 683; do
  body="/tmp/vf-delete-$id.json"
  code=$(curl -sS -o "$body" -w '%{http_code}' \
    -X DELETE -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$id?delay=0")
  echo "server=$id http=$code body=$(tr '\n' ' ' <"$body")"
done

echo "=== Stop stale retries and requeue DE/GB completion ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.instances
SET state='deleted', billing_status='cancelled', external_id=NULL, ip_address=NULL,
    next_billing_at=NULL,
    provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      || jsonb_build_object('cleanup_reason','stale_delete_pending','cleaned_at',now()),
    worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id='d388fad7-96c6-4d28-a679-d09142d02973';

UPDATE vps.instances
SET state='error', billing_status='cancelled', external_id=NULL, ip_address=NULL,
    next_billing_at=NULL,
    provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      || jsonb_build_object('provision_error','VirtFusion template is invalid; stale shell removed','provision_failed_at',now()),
    worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id='6677945d-15e2-46e1-b36b-48dd84e61c0b';

UPDATE vps.instances
SET state='creating', external_id=NULL, ip_address=NULL,
    provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      - 'provision_error' - 'provision_failed_at' - 'guest_agent_warmup_at',
    worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id='eda519e1-afdc-47cc-a9e7-502dbbcfc146';

UPDATE vps.instances
SET provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      || jsonb_build_object('guest_agent_warmup_at',to_jsonb(now() - interval '2 minutes')),
    worker_poll_claimed_at=NULL, worker_poll_claimed_by=NULL, updated_at=now()
WHERE id='7bf455ad-45a2-489a-ae13-bea4260c6844' AND state='creating';
SQL

docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null ||
  docker restart docker-vps-worker-1
echo "backup=$BACKUP_DIR"
