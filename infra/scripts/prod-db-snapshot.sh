#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker && set -a && source .env && set +a
echo "=== ERROR INSTANCE ==="
psql "$POSTGRES_DSN" -c "SELECT id, region, state FROM vps.instances WHERE state = 'error';"
echo "=== 7-DAY BILLING RUNNING ==="
psql "$POSTGRES_DSN" -c "SELECT id, billing_period_days, auto_renew, next_billing_at::date FROM vps.instances WHERE billing_period_days = 7 AND state = 'running';"
