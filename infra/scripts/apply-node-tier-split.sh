#!/usr/bin/env bash
# Apply canonical node tier split (DE-1 prosto off, DE-mid midrange only).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
bash "$ROOT/infra/scripts/migrate.sh"
echo "=== tiers after migration ==="
source "$ROOT/infra/docker/.env"
psql "$POSTGRES_DSN" -c "SELECT name, supported_tiers FROM vps.nodes WHERE name IN ('DE-1','DE-mid','FI-1','GB-1','NL-1') ORDER BY name;"
