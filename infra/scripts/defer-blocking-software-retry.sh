#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
source .env
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 -c "
UPDATE vps.instances
SET provider_meta=COALESCE(provider_meta,'{}'::jsonb)
      || jsonb_build_object('software_install_retry_at',to_jsonb((now() + interval '24 hours')::text)),
    updated_at=now()
WHERE id='9c852b64-1f35-4bad-88b8-43c67a9e60da'
  AND NULLIF(provider_meta->>'software_install_error','') IS NOT NULL;"
