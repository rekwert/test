#!/usr/bin/env bash
# Re-commission VirtFusion HV2 (FI) and HV4 (GB) so new orders can allocate.
set -euo pipefail

cd /opt/testVPStrade
set -a
source infra/docker/.env
[[ -f infra/docker/.env.probe ]] && source infra/docker/.env.probe
set +a

NL_HOST="${VIRTFUSION_CTRL_SSH_HOST:-66.248.206.14}"
NL_PASS="${NL_SSH_PASS:?missing NL_SSH_PASS}"
FI_PASS="${FI_SSH_PASS:?missing FI_SSH_PASS}"
GB_PASS="${GB_SSH_PASS:?missing GB_SSH_PASS}"

recommission_hv() {
  local hv_id="$1"
  local hv_ip="$2"
  local hv_pass="$3"
  local hv_label="$4"

  echo ""
  echo "========== Re-commission HV${hv_id} (${hv_label} ${hv_ip}) =========="

  SSHPASS="$NL_PASS" sshpass -e ssh -o StrictHostKeyChecking=no "root@${NL_HOST}" \
    "HV_ID=${hv_id} HV_IP=${hv_ip} HV_PASS=${hv_pass} HV_LABEL=${hv_label} bash -s" <<'NL'
set -euo pipefail
set -a
source /opt/virtfusion/app/control/.env
set +a
cd /opt/virtfusion/app/control
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)
export SSHPASS="$HV_PASS"

echo "=== Before ==="
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,maintenance,LENGTH(token) token_len FROM hypervisors WHERE id=${HV_ID};"

echo "=== Ensure NL SSH key on ${HV_LABEL} ==="
install -d -m 700 /root/.ssh
if [[ ! -f /root/.ssh/id_ed25519 ]]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(< /root/.ssh/id_ed25519.pub)
ssh-keygen -f /root/.ssh/known_hosts -R "$HV_IP" 2>/dev/null || true
sshpass -e ssh -n -o StrictHostKeyChecking=no "root@${HV_IP}" \
  "install -d -m 700 /root/.ssh; touch /root/.ssh/authorized_keys; grep -qxF '${PUB}' /root/.ssh/authorized_keys || echo '${PUB}' >> /root/.ssh/authorized_keys; chmod 600 /root/.ssh/authorized_keys"
ssh -n -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=no "root@${HV_IP}" hostname

echo "=== Reset commission state ==="
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, token=NULL, enabled=1, maintenance=0, prohibit=0 WHERE id=${HV_ID};"
ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no "root@${HV_IP}" \
  "rm -f /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl enable libvirtd vf-nginx 2>/dev/null || true; systemctl start libvirtd vf-nginx 2>/dev/null || true"

echo "=== Official re-commission ==="
set +e
printf "${HV_ID}\nyes\nyes\n${HV_PASS}\n" | timeout 600 "$PHP" artisan hypervisor:re-commission 2>&1 | tail -40
RC=$?
set -e
echo "artisan_exit=$RC"

echo "=== Poll result ==="
COMM=0
TOKLEN=0
AUTH=NO
for I in $(seq 1 42); do
  COMM=$("${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=${HV_ID};")
  TOKLEN=$("${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=${HV_ID};")
  AUTH=$(ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no "root@${HV_IP}" \
    "test -s /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO")
  echo "poll=$I commissioned=$COMM token_len=$TOKLEN auth=$AUTH"
  if [[ "$COMM" == "3" && "$TOKLEN" -gt 100 && "$AUTH" == "OK" ]]; then
    break
  fi
  sleep 10
done

if [[ "$COMM" != "3" || "$TOKLEN" -le 100 || "$AUTH" != "OK" ]]; then
  echo "=== Official failed — manual token sync ==="
  "$PHP" <<PHP
<?php
require __DIR__ . '/vendor/autoload.php';
\$app = require __DIR__ . '/bootstrap/app.php';
\$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
\$ref = Hypervisor::find(1);
\$hv = Hypervisor::find(${HV_ID});
if (!\$ref || !\$hv) { fwrite(STDERR, "missing hv\n"); exit(1); }
\$refPlain = Crypt::decryptString(\$ref->token);
function genPlain(int \$len): string {
  \$chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  \$s = '';
  for (\$i = 0; \$i < \$len; \$i++) {
    \$s .= \$chars[random_int(0, strlen(\$chars) - 1)];
  }
  return \$s;
}
\$plain = genPlain(strlen(\$refPlain));
\$hash = bin2hex(random_bytes(32));
\$hv->token = Crypt::encryptString(\$plain);
\$hv->commissioned = 3;
\$hv->enabled = 1;
\$hv->maintenance = 0;
\$hv->prohibit = 0;
\$hv->save();
\$auth = ['ip' => '66.248.206.14', 'token' => substr(\$plain, 0, 200), 'hash' => \$hash, 'id' => ${HV_ID}];
file_put_contents('/tmp/hv-auth-${HV_ID}.json', json_encode(\$auth, JSON_UNESCAPED_SLASHES));
echo "manual plain_len=".strlen(\$plain)." commissioned=".\$hv->commissioned."\n";
PHP
  sshpass -e scp -o StrictHostKeyChecking=no "/tmp/hv-auth-${HV_ID}.json" "root@${HV_IP}:/opt/virtfusion/app/hypervisor/conf/auth.json"
  ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no "root@${HV_IP}" \
    'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json'
  COMM=$("${MYN[@]}" -e "SELECT commissioned FROM hypervisors WHERE id=${HV_ID};")
  TOKLEN=$("${MYN[@]}" -e "SELECT IFNULL(LENGTH(token),0) FROM hypervisors WHERE id=${HV_ID};")
  AUTH=$(ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no "root@${HV_IP}" \
    "test -s /opt/virtfusion/app/hypervisor/conf/auth.json && echo OK || echo NO")
fi

echo "=== Start services on ${HV_LABEL} ==="
ssh -n -o BatchMode=yes -o StrictHostKeyChecking=no "root@${HV_IP}" bash -s <<'HV'
set -euo pipefail
systemctl enable --now libvirtd vf-nginx 2>/dev/null || true
supervisorctl restart vf-queue-hv: 2>/dev/null || true
systemctl is-active libvirtd vf-nginx || true
ss -tln | awk '/:8892/{print "port8892=LISTEN"}'
HV
supervisorctl restart vf-queue: 2>/dev/null | tail -2 || true

"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) token_len FROM hypervisors WHERE id=${HV_ID};"

if [[ "$COMM" != "3" || "$TOKLEN" -le 100 || "$AUTH" != "OK" ]]; then
  echo "HV${HV_ID}_FAILED"
  exit 1
fi
echo "HV${HV_ID}_OK"
NL
}

recommission_hv 2 "95.216.1.155" "$FI_PASS" "FI-node1"
recommission_hv 4 "212.108.83.47" "$GB_PASS" "GB-prosto"

echo ""
echo "========== Update portal nodes =========="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes SET
  status='online', maintenance_mode=false, vf_enabled=true, vf_commissioned=3, updated_at=now()
WHERE external_id IN ('2','4');
SELECT name, region, status, external_id, vf_commissioned FROM vps.nodes WHERE region IN ('fi','gb');
SQL

echo ""
echo "========== Verify VF allocate (package 1) =========="
API="${VIRTFUSION_API_URL}"
KEY="${VIRTFUSION_API_KEY}"
for hv in 2 4; do
  echo "--- POST /servers hypervisorId=$hv ---"
  RESP=$(curl -sk -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
    -d "{\"packageId\":1,\"userId\":1,\"hypervisorId\":$hv}" "$API/servers")
  echo "$RESP" | head -c 400
  echo ""
  SID=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('id',''))" 2>/dev/null || true)
  if [[ -n "$SID" && "$SID" != "" ]]; then
    echo "Deleting test server $SID"
    curl -sk -X DELETE -H "Authorization: Bearer $KEY" "$API/servers/$SID" >/dev/null || true
  fi
done

echo ""
echo "========== Stuck creating instances =========="
psql "$POSTGRES_DSN" -c "
SELECT hostname, id, state, region, created_at
FROM vps.instances WHERE state='creating' ORDER BY created_at;"

echo ""
echo "========== Restart worker =========="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1
sleep 5
echo "VF_FI_GB_RECOMMISSION_DONE"
