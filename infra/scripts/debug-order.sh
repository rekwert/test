#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env

USER_ID="e160cfad-6859-41e4-951f-2cedc17b6479"
PLAN_ID="11111111-1111-1111-1111-111111111101"

psql "$POSTGRES_DSN" <<SQL
\set ON_ERROR_STOP on
BEGIN;

SELECT name, price_monthly::float8 FROM vps.plans WHERE id = '$PLAN_ID'::uuid AND active = true;

SELECT COALESCE(a.billing_status, 'active'), COALESCE(a.balance, 0)::float8
FROM billing.accounts a WHERE a.user_id = '$USER_ID'::uuid FOR UPDATE;

SELECT n.id::text
FROM vps.nodes n
WHERE n.region = 'nl'
  AND n.status = 'online'
  AND COALESCE(n.vf_enabled, true) = true
  AND COALESCE(n.maintenance_mode, false) = false
  AND (
    SELECT COUNT(*)::int FROM vps.instances i
    WHERE i.node_id = n.id AND i.state <> 'deleted'
  ) < n.capacity_instances
ORDER BY (
  SELECT COUNT(*)::int FROM vps.instances i
  WHERE i.node_id = n.id AND i.state <> 'deleted'
) ASC, n.name ASC
LIMIT 1;

SELECT public_key FROM auth.ssh_keys WHERE user_id = '$USER_ID'::uuid ORDER BY created_at DESC LIMIT 10;

ROLLBACK;
SQL
