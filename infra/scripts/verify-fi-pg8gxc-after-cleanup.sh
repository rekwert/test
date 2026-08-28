#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade/infra/docker
set -a; source .env; source .env.probe; set +a

psql "$POSTGRES_DSN" -x -c "
SELECT hostname,state,external_id,ip_address,provider_meta,updated_at
FROM vps.instances WHERE hostname='vps-pg8gxc';"

EXT=$(psql "$POSTGRES_DSN" -Atc "SELECT COALESCE(external_id,'') FROM vps.instances WHERE hostname='vps-pg8gxc'")
if [[ -n "$EXT" ]]; then
  curl -fsS -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
    "${VIRTFUSION_API_URL%/}/servers/$EXT" |
    python3 -c 'import json,sys
d=(json.load(sys.stdin).get("data") or {})
ips=[x.get("address") for i in (d.get("network") or {}).get("interfaces",[]) for x in (i.get("ipv4") or [])]
print("vf_id=",d.get("id"),"state=",d.get("state"),"buildFailed=",d.get("buildFailed"),"ips=",ips,"agent=",bool(d.get("qemuAgent")))'
fi

echo "=== FI domains ==="
SSHPASS="$FI_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@95.216.1.155 \
  'virsh list --all'

echo "=== recent matching worker logs ==="
docker logs --since 5m docker-vps-worker-1 2>&1 |
  python3 -c 'import re,sys
p=re.compile(r"pg8gxc|b95ac26b|vf=688|server.?688",re.I)
print("".join(x for x in sys.stdin if p.search(x)),end="")'
