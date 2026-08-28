#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
source .env
psql "$POSTGRES_DSN" -x -c "
SELECT id,hostname,region,state,external_id,ip_address,node_id,provider_meta,updated_at
FROM vps.instances
WHERE id IN (
 'd388fad7-96c6-4d28-a679-d09142d02973',
 '6677945d-15e2-46e1-b36b-48dd84e61c0b',
 '9c852b64-1f35-4bad-88b8-43c67a9e60da',
 '0b7f964a-6336-4f43-ac61-3b6d995534d7',
 '2f22d8f2-72b5-438c-a2a7-a6342b42fdd0',
 'b95ac26b-09f8-4cfb-afe3-6cc05f24165e'
) ORDER BY created_at;"

echo "=== all creating instances ==="
psql "$POSTGRES_DSN" -c "
SELECT id,hostname,region,external_id,ip_address,updated_at
FROM vps.instances WHERE state='creating'
ORDER BY updated_at,created_at;"
