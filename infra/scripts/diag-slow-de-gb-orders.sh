#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a; source .env; set +a

psql "$POSTGRES_DSN" -x -c "
SELECT i.id,i.hostname,i.region,i.state,i.external_id,i.ip_address,i.node_id,
       i.provider_meta,i.worker_poll_claimed_at,i.updated_at,
       o.order_number,o.os_template_id,n.name node,n.status node_status,n.maintenance_mode
FROM vps.instances i
LEFT JOIN vps.orders o ON o.id=i.order_id
LEFT JOIN vps.nodes n ON n.id=i.node_id
WHERE i.hostname IN ('vps-ac8owm','vps-sv9f8j')
ORDER BY i.created_at;"

psql "$POSTGRES_DSN" -x -c "
SELECT id,event_type,payload,published,worker_poll_claimed_at,worker_poll_claimed_by,created_at
FROM vps.outbox
WHERE payload->>'hostname' IN ('vps-ac8owm','vps-sv9f8j')
ORDER BY id;"

echo "=== VirtFusion state ==="
while read -r host ext; do
  [[ -n "$ext" ]] || continue
  printf '%s vf=%s ' "$host" "$ext"
  curl -fsS -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$ext" |
    python3 -c 'import json,sys
d=(json.load(sys.stdin).get("data") or {})
ips=[x.get("address") for i in (d.get("network") or {}).get("interfaces",[]) for x in (i.get("ipv4") or [])]
print("state=",d.get("state"),"failed=",d.get("buildFailed"),"tasks=",d.get("tasks"),"ips=",ips)'
done < <(psql "$POSTGRES_DSN" -AtF' ' -c "
  SELECT hostname,COALESCE(external_id,'') FROM vps.instances
  WHERE hostname IN ('vps-ac8owm','vps-sv9f8j')")

echo "=== recent worker logs ==="
docker logs --since 30m docker-vps-worker-1 2>&1 |
  python3 -c 'import re,sys
p=re.compile(r"ac8owm|sv9f8j|provision|retry build|software retry|reinstall software",re.I)
print("".join(x for x in sys.stdin if p.search(x)),end="")'
