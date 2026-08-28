#!/bin/bash
set -euo pipefail
: "${POSTGRES_DSN:?set POSTGRES_DSN to portal database connection string}"

psql "$POSTGRES_DSN" -c "UPDATE vps.nodes SET external_id='2', status='online', vf_enabled=true, maintenance_mode=false, updated_at=now() WHERE region='nl'"
psql "$POSTGRES_DSN" -c "SELECT name, region, status, external_id, vf_enabled FROM vps.nodes"
