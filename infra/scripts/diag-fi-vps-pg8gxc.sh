#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a; source .env; set +a

psql "$POSTGRES_DSN" -x -c "
SELECT i.*,o.order_number,n.name node,n.status node_status,n.maintenance_mode
FROM vps.instances i
LEFT JOIN vps.orders o ON o.id=i.order_id
LEFT JOIN vps.nodes n ON n.id=i.node_id
WHERE i.hostname='vps-pg8gxc';"

echo "=== provision outbox ==="
psql "$POSTGRES_DSN" -x -c "
SELECT * FROM vps.outbox
WHERE payload->>'instance_id'='b95ac26b-09f8-4cfb-afe3-6cc05f24165e'
ORDER BY id DESC;"

EXT=$(psql "$POSTGRES_DSN" -Atc "SELECT COALESCE(external_id,'') FROM vps.instances WHERE hostname='vps-pg8gxc'")
if [[ -n "$EXT" ]]; then
  echo "=== VirtFusion server $EXT ==="
  curl -fsS -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$EXT" |
    python3 -m json.tool
fi

echo "=== matching worker logs ==="
docker logs --since 8h docker-vps-worker-1 2>&1 |
  python3 -c 'import re,sys
p=re.compile(r"pg8gxc|provision|FI-1|95\.216\.1\.155",re.I)
print("".join(line for line in sys.stdin if p.search(line)),end="")' || true
