#!/usr/bin/env bash
# Finish GB (UK) prosto hypervisor: VF group, SSH, IP pool, portal node, worker regions.
# Run on back server (198.13.189.75):
#   export GB_SSH_PASS='...'
#   bash /tmp/vf-fix-gb-prosto-finish.sh
set -euo pipefail

GB_GROUP_ID="${GB_GROUP_ID:-4}"
GB_HV_IP="${GB_HV_IP:-212.108.83.47}"
GB_HV_ID="${GB_HV_ID:-4}"
GB_NET_ID="${GB_NET_ID:-4}"
NL_SSH_PASS="${NL_SSH_PASS:-zx_zvJdI9P}"
: "${GB_SSH_PASS:?set GB_SSH_PASS}"

GB_POOL_GATEWAY="${GB_POOL_GATEWAY:-172.24.5.1}"
GB_POOL_NETMASK="${GB_POOL_NETMASK:-255.255.255.0}"
GB_POOL_START="${GB_POOL_START:-172.24.5.159}"
GB_POOL_END="${GB_POOL_END:-172.24.5.174}"
GB_BLK_NAME="${GB_BLK_NAME:-GB public UK}"

echo "=== 1. GB host: ip_forward + vf-data ==="
SSHPASS="$GB_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "root@${GB_HV_IP}" 'bash -s' <<'REMOTE'
set -euo pipefail
mkdir -p /home/vf-data/disk
chmod 755 /home/vf-data /home/vf-data/disk
cat >/etc/sysctl.d/99-vf-gb.conf <<'SYS'
net.ipv4.ip_forward=1
SYS
sysctl -p /etc/sysctl.d/99-vf-gb.conf
sysctl net.ipv4.ip_forward
REMOTE

echo "=== 2. VF NL: run vf-fix-gb-vf-db.sh ==="
SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  /opt/testVPStrade/infra/scripts/vf-fix-gb-vf-db.sh root@66.248.206.14:/tmp/vf-fix-gb-vf-db.sh 2>/dev/null \
  || SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  /tmp/vf-fix-gb-vf-db.sh root@66.248.206.14:/tmp/vf-fix-gb-vf-db.sh
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-fix-gb-vf-db.sh && GB_SSH_PASS='$GB_SSH_PASS' bash /tmp/vf-fix-gb-vf-db.sh"

echo "=== 3. Portal: GB-1 node + provision regions ==="
cd /opt/testVPStrade/infra/docker
set -a; source .env; set +a
python3 <<PY
import os,re,subprocess,urllib.parse
dsn=os.environ['POSTGRES_DSN']
m=re.match(r'postgres(?:ql)?://([^:]+):([^@]+)@([^:/]+):?(\d+)?/([^?]+)', dsn)
user,pwd,host,port,db=m.groups(); port=port or '5432'
env=os.environ.copy(); env['PGPASSWORD']=urllib.parse.unquote(pwd)
ext=os.environ.get('GB_GROUP_ID','4')
sql=f"""
INSERT INTO vps.nodes (id, name, region, external_id, status, capacity_instances, supported_tiers, vf_enabled, vf_commissioned, vf_ip, vf_name)
VALUES (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb004',
  'GB-1',
  'gb',
  '{ext}',
  'online',
  30,
  ARRAY['prosto','midrange','hustle']::text[],
  true,
  3,
  '${GB_HV_IP}',
  'GB-prosto'
)
ON CONFLICT (id) DO UPDATE SET
  name=EXCLUDED.name, region=EXCLUDED.region, external_id=EXCLUDED.external_id,
  status=EXCLUDED.status, capacity_instances=EXCLUDED.capacity_instances,
  supported_tiers=EXCLUDED.supported_tiers, vf_enabled=EXCLUDED.vf_enabled,
  vf_commissioned=EXCLUDED.vf_commissioned, vf_ip=EXCLUDED.vf_ip, vf_name=EXCLUDED.vf_name,
  updated_at=now();
UPDATE vps.regions SET enabled=true WHERE code='gb';
"""
subprocess.run(['psql','-h',host,'-p',port,'-U',user,'-d',db,'-c',sql],env=env,check=True)
subprocess.run(['psql','-h',host,'-p',port,'-U',user,'-d',db,'-c',
  "SELECT r.code,r.enabled,(SELECT COUNT(*) FROM vps.nodes n WHERE n.region=r.code AND n.status='online') nodes FROM vps.regions r ORDER BY sort_order;"],env=env,check=True)
subprocess.run(['psql','-h',host,'-p',port,'-U',user,'-d',db,'-c',
  "SELECT name,region,status,external_id,supported_tiers,vf_ip FROM vps.nodes ORDER BY region;"],env=env,check=True)
PY

CUR=$(grep '^VIRTFUSION_PROVISION_REGIONS=' .env | cut -d= -f2-)
if echo "$CUR" | grep -qiE '(^|,)gb(,|$)'; then
  echo "gb already in VIRTFUSION_PROVISION_REGIONS"
else
  sed -i "s|^VIRTFUSION_PROVISION_REGIONS=.*|VIRTFUSION_PROVISION_REGIONS=${CUR},gb|" .env
fi
grep '^VIRTFUSION_PROVISION_REGIONS=' .env

docker compose -f docker-compose.back.yml --env-file .env up -d vps-worker vps
sleep 3

echo "=== 4. API regions check ==="
curl -sk -H "Accept: application/json" http://127.0.0.1:8080/v1/regions 2>/dev/null | python3 -c "
import sys,json
d=json.load(sys.stdin)
for r in d.get('data',d if isinstance(d,list) else []):
    if isinstance(r,dict) and r.get('code') in ('gb','de','nl','fi'):
        print(r.get('code'), 'enabled=', r.get('enabled'), 'available=', r.get('available'))
" 2>/dev/null || echo "(regions API skip)"

echo VF_GB_PROSTO_FINISH_DONE
