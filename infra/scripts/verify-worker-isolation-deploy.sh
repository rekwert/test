#!/usr/bin/env bash
set -euo pipefail

cd /opt/testVPStrade/infra/docker
set -a
source .env
set +a

echo "=== active provisioning ==="
psql "$POSTGRES_DSN" -c "
SELECT i.id, i.hostname, i.region, i.state, i.external_id,
       host(i.ip_address) AS ip, i.updated_at, n.name AS node
FROM vps.instances i
LEFT JOIN vps.nodes n ON n.id = i.node_id
WHERE i.hostname = 'vps-v95juk'
   OR i.state IN ('queued', 'creating', 'reinstalling')
ORDER BY i.created_at;"

echo "=== pending outbox ==="
psql "$POSTGRES_DSN" -c "
SELECT event_type, count(*)
FROM vps.outbox
WHERE published = false
GROUP BY event_type
ORDER BY count(*) DESC;"

echo "=== containers ==="
docker inspect docker-vps-1 docker-vps-worker-1 \
  --format '{{.Name}} {{.Config.Image}} {{.State.Status}}'

echo "=== worker logs ==="
docker logs docker-vps-worker-1 --since 10m 2>&1
