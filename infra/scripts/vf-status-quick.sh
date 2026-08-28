#!/bin/bash
set -euo pipefail
: "${NL_PASS:?set NL_PASS to NL root password}"

echo "=== PANEL FROM BACK ==="
curl -sk -o /dev/null -w "login:%{http_code} time:%{time_total}\n" --connect-timeout 10 https://66.248.206.14/login || echo "login:FAIL"
curl -sk -o /dev/null -w "dashboard:%{http_code}\n" --connect-timeout 10 https://66.248.206.14/admin/dashboard || echo "dashboard:FAIL"

echo ""
echo "=== ON NL NODE ==="
sshpass -p "$NL_PASS" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=12 root@66.248.206.14 bash <<'EOF'
echo "services:"
systemctl is-active vf-nginx vf-php8-fpm libvirtd 2>/dev/null || true
echo "ports:"
ss -tlnp | grep -E ':443 |:8892 ' || true
echo "local_login:"
curl -sk -o /dev/null -w "%{http_code}\n" https://127.0.0.1/login || echo FAIL
echo "br0:"
ip -br addr show br0 2>/dev/null || echo no-br0
echo "commissioned:"
source /opt/virtfusion/app/control/.env 2>/dev/null || true
mysql -N -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT id,name,enabled,commissioned FROM hypervisors;" 2>/dev/null || echo "no-db-or-empty"
mysql -N -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT LEFT(value,40) FROM configuration WHERE \`key\`='licenseKey';" 2>/dev/null || true
EOF

echo ""
echo "=== API FROM BACK ENV ==="
set -a
source /opt/testVPStrade/infra/docker/.env
set +a
echo "VIRTFUSION_API_URL=$VIRTFUSION_API_URL"
echo "VIRTFUSION_API_KEY_len=${#VIRTFUSION_API_KEY}"
echo "VIRTFUSION_USE_MOCK=$VIRTFUSION_USE_MOCK"

code=$(curl -sk -o /tmp/vf_hv.json -w "%{http_code}" --connect-timeout 15 \
  -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  "${VIRTFUSION_API_URL}/compute/hypervisors/1" || echo 000)
echo "GET hypervisors/1 -> HTTP $code"
head -c 500 /tmp/vf_hv.json 2>/dev/null; echo

code2=$(curl -sk -o /tmp/vf_post.json -w "%{http_code}" --connect-timeout 20 \
  -X POST -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"packageId":1,"userId":1,"hypervisorId":1}' \
  "${VIRTFUSION_API_URL}/servers" || echo 000)
echo "POST /servers -> HTTP $code2"
head -c 400 /tmp/vf_post.json 2>/dev/null; echo

echo ""
echo "=== WORKER TAIL ==="
docker logs docker-vps-worker-1 --tail 20 2>&1
