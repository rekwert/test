#!/bin/bash
set -a
source /opt/testVPStrade/infra/docker/.env
set +a

# Prefer POSTGRES_DSN if set
DSN="${POSTGRES_DSN:-}"
if [ -z "$DSN" ]; then
  echo "no POSTGRES_DSN"
  exit 1
fi

echo "=== RECENT INVOICES ==="
docker run --rm --network docker_default postgres:16-alpine \
  psql "$DSN" -c "SELECT id, provider, status, amount, robokassa_inv_id, created_at FROM billing.invoices ORDER BY created_at DESC LIMIT 20;"

echo ""
echo "=== ROBOKASSA PENDING/FAILED ==="
docker run --rm --network docker_default postgres:16-alpine \
  psql "$DSN" -c "SELECT id, status, amount, robokassa_inv_id, LEFT(COALESCE(payment_url,''),100) AS payment_url, created_at FROM billing.invoices WHERE provider = 'robokassa' ORDER BY created_at DESC LIMIT 20;"

echo ""
echo "=== BALANCE LEDGER last 10 ==="
docker run --rm --network docker_default postgres:16-alpine \
  psql "$DSN" -c "SELECT * FROM billing.balance_transactions ORDER BY created_at DESC LIMIT 10;" 2>&1 | head -40

echo ""
echo "=== GATEWAY ACCESS since restart (grep pay) ==="
docker logs docker-gateway-1 --since 2h 2>&1 | grep -iE 'topup|robo|webhook|/billing' | tail -40

echo ""
echo "=== TEST CREATE TOPUP endpoint via gateway locally ==="
# just show routes work
curl -s -o /tmp/billready -w "billing_ready:%{http_code}\n" http://127.0.0.1:8002/health || \
  docker exec docker-gateway-1 wget -qO- http://billing:8002/health 2>/dev/null | head -c 300
echo

echo "=== PUBLIC webhook path check ==="
# ResultURL typically /api/v1/webhooks/robokassa
curl -sk -o /dev/null -w "front_webhook:%{http_code}\n" -X POST "https://cloud-hustle.com/api/v1/webhooks/robokassa" -d "OutSum=1&InvId=1&SignatureValue=x" || true
curl -s -o /dev/null -w "local_webhook:%{http_code}\n" -X POST "http://127.0.0.1:8080/api/v1/webhooks/robokassa" -d "OutSum=1&InvId=1&SignatureValue=x" || \
  docker exec docker-gateway-1 wget -qO- --post-data='OutSum=1&InvId=1&SignatureValue=x' http://billing:8002/api/v1/webhooks/robokassa 2>&1 | head -c 200
echo
