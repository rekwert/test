#!/usr/bin/env bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
INSTANCE_ID="8d8a46f7-c421-4fb8-b540-c0932299df95"

echo "=== Retire failed VirtFusion shells on HV5 only ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "SELECT id,name,state,commissioned,deleted_at FROM servers WHERE hypervisor_id=5 AND deleted_at IS NULL ORDER BY id;"
"${MY[@]}" -e "UPDATE servers SET state='deleted', deleted_at=NOW() WHERE hypervisor_id=5 AND deleted_at IS NULL AND state IN ('failed','allocated');"
"${MY[@]}" -e "SELECT ROW_COUNT() retired_hv5_failed_shells;"
supervisorctl restart vf-queue:
NL

echo "=== Restore portal HV5 health and retry only vps-gm4kgz ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<SQL
UPDATE vps.nodes
SET external_id = '5',
    vf_name = 'DE-midrange',
    vf_ip = '66.151.40.165',
    vf_commissioned = 3,
    vf_enabled = true,
    status = 'online',
    maintenance_mode = false,
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

UPDATE vps.instances
SET external_id = NULL,
    ip_address = NULL,
    state = 'creating',
    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
      - 'provision_error'
      - 'provision_failed_at'
      - 'guest_agent_warmup_at'
      - 'vf_password_reset_at'
      - 'provision_allocate_claim',
    worker_poll_claimed_at = NULL,
    worker_poll_claimed_by = NULL,
    updated_at = now()
WHERE id = '${INSTANCE_ID}'
  AND node_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

SELECT i.hostname, i.state, i.external_id, i.ip_address::text,
       n.name AS node, n.external_id AS hypervisor_id, n.vf_ip
FROM vps.instances i
JOIN vps.nodes n ON n.id = i.node_id
WHERE i.id = '${INSTANCE_ID}';
SQL

echo "=== Restart VPS worker ==="
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null ||
  docker restart docker-vps-worker-1
echo "DE_MID_RETRY_STARTED"
