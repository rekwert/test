#!/bin/bash
source /opt/testVPStrade/infra/docker/.env
EXT=$(psql "$POSTGRES_DSN" -t -A -c "SELECT COALESCE(external_id,'') FROM vps.instances WHERE hostname='vps-gm4kgz';")
echo "portal ext=$EXT"
psql "$POSTGRES_DSN" -c "SELECT hostname,state,external_id,ip_address::text FROM vps.instances WHERE hostname='vps-gm4kgz';"
if [[ -n "$EXT" ]]; then
  curl -sk "${VIRTFUSION_API_URL}/servers/${EXT}" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
    | python3 -c 'import sys,json;s=json.load(sys.stdin)["data"];print("vf",s["id"],"state",s["state"],"commission",s["commissionStatus"],"built",s["built"],"ip",s["network"]["interfaces"][0]["ipv4"] if s.get("network",{}).get("interfaces") else None)'
fi
docker logs docker-vps-worker-1 --since 5m 2>&1 | grep -iE 'gm4kgz|8d8a46f7|677|678|complete provision|retry build' | tail -30
