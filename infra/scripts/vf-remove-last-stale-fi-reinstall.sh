#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
source .env

psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
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
SQL

docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null ||
  docker restart docker-vps-worker-1
