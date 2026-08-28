#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a
source .env
set +a
echo "=== free week billing_period_days ==="
psql "$POSTGRES_DSN" -c "
SELECT billing_period_days, count(*)
FROM vps.instances
WHERE COALESCE((provider_meta->>'free_week')::boolean, false)
   OR COALESCE((provider_meta->>'trial')::boolean, false)
GROUP BY 1 ORDER BY 1;"
echo "=== sample trial instances ==="
psql "$POSTGRES_DSN" -c "
SELECT o.order_number, i.billing_period_days,
       i.provider_meta->>'initial_prepaid_days' AS prepaid,
       i.next_billing_at, i.auto_renew
FROM vps.instances i
JOIN vps.orders o ON o.id = i.order_id
WHERE COALESCE((provider_meta->>'free_week')::boolean, false)
ORDER BY o.order_number DESC LIMIT 5;"
