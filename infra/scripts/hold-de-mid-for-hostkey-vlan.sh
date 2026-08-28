#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes
SET status = 'maintenance',
    maintenance_mode = true,
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';

UPDATE vps.instances
SET state = 'queued',
    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
      || jsonb_build_object(
        'wait_reason', 'hostkey_vlan',
        'hostkey_ticket', 'CS-522770'
      ),
    worker_poll_claimed_at = NULL,
    worker_poll_claimed_by = NULL,
    updated_at = now()
WHERE id = '8d8a46f7-c421-4fb8-b540-c0932299df95'
  AND state = 'creating';

SELECT i.hostname, i.state, i.external_id, i.ip_address::text,
       n.name, n.status, n.maintenance_mode
FROM vps.instances i
JOIN vps.nodes n ON n.id = i.node_id
WHERE i.id = '8d8a46f7-c421-4fb8-b540-c0932299df95';
SQL
