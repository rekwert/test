#!/bin/bash
# One-off: sync portal DB instance rows with SolusVM after manual IP pool fixes.
# Usage: POSTGRES_DSN='postgres://...' bash infra/scripts/fix-portal-ips.sh
set -euo pipefail
: "${POSTGRES_DSN:?set POSTGRES_DSN to portal database connection string}"

psql "$POSTGRES_DSN" << 'SQL'
SELECT id, hostname, external_id, host(ip_address)::text AS ip_address, state FROM vps.instances WHERE state != 'deleted' ORDER BY created_at DESC LIMIT 10;

-- Edit hostname → (external_id, ip) pairs for your environment before running.
-- UPDATE vps.instances SET external_id = '165', ip_address = '66.248.206.61'::inet, state = 'running'
--   WHERE hostname = 'vps-example' AND state != 'deleted';

UPDATE vps.outbox SET status = 'published', processed_at = now() WHERE status != 'published';

SELECT id, hostname, external_id, host(ip_address)::text AS ip_address, state FROM vps.instances WHERE state != 'deleted' ORDER BY created_at DESC LIMIT 10;
SQL
