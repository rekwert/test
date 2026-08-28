#!/bin/bash
set -a
source /opt/testVPStrade/infra/docker/.env
set +a

echo "=== How gateway proxies webhooks ==="
docker exec docker-gateway-1 wget -S -O- --post-data='OutSum=50.00&InvId=100025&SignatureValue=deadbeef' \
  http://billing:8002/api/v1/webhooks/robokassa 2>&1 | head -30
echo "---"
docker exec docker-gateway-1 wget -S -O- --post-data='OutSum=50.00&InvId=100025&SignatureValue=deadbeef' \
  http://billing:8002/robokassa 2>&1 | head -30

echo ""
echo "=== Via gateway container ports ==="
docker port docker-gateway-1 2>/dev/null || true
docker exec docker-gateway-1 wget -S -O- --post-data='OutSum=50.00&InvId=100025&SignatureValue=deadbeef' \
  http://127.0.0.1:8080/api/v1/webhooks/robokassa 2>&1 | head -40

echo ""
echo "=== Full payment URL of last invoice ==="
docker run --rm --network docker_default postgres:16-alpine \
  psql "$POSTGRES_DSN" -t -A -c "SELECT payment_url FROM billing.invoices WHERE id='ca589986-4a35-4932-92b4-2791b035d181';"

echo ""
echo "=== Public front -> api path ==="
curl -sk -D- -o /tmp/wh_body -X POST "https://cloud-hustle.com/api/v1/webhooks/robokassa" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "OutSum=50.00&InvId=100025&SignatureValue=deadbeef"
echo
head -c 300 /tmp/wh_body; echo

curl -sk -D- -o /tmp/wh_body2 -X POST "https://api.cloud-hustle.com/api/v1/webhooks/robokassa" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "OutSum=50.00&InvId=100025&SignatureValue=deadbeef" 2>&1 | head -20
echo
head -c 200 /tmp/wh_body2; echo

echo ""
echo "=== FRONT server proxy check ==="
ssh -o StrictHostKeyChecking=no -o BatchMode=yes -o ConnectTimeout=8 root@213.148.3.172 \
  'docker ps --format "{{.Names}}"; grep -R "webhook\|billing\|8080\|api" /opt -g "*.yml" 2>/dev/null | head -20' 2>&1 | head -40
