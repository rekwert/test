#!/bin/bash
set -a
source /opt/testVPStrade/infra/docker/.env
set +a
docker run --rm --network docker_default postgres:16-alpine psql "$POSTGRES_DSN" -c \
  "SELECT id, name, region, status, external_id, COALESCE(vf_enabled,true) AS vf_enabled, COALESCE(maintenance_mode,false) AS maint FROM vps.nodes ORDER BY region;"
echo "---"
grep VIRTFUSION_PROVISION /opt/testVPStrade/infra/docker/.env || true
