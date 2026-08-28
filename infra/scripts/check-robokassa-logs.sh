#!/bin/bash
set -euo pipefail

ENV=/opt/testVPStrade/infra/docker/.env

echo "=== ROBOKASSA CONFIG ==="
while IFS= read -r line; do
  case "$line" in
    \#*|'') continue ;;
  esac
  k="${line%%=*}"
  v="${line#*=}"
  case "$k" in
    ROBOKASSA_ENABLED|ROBOKASSA_TEST_MODE|ROBOKASSA_MERCHANT_LOGIN|ROBOKASSA_SUCCESS_URL|ROBOKASSA_FAIL_URL)
      echo "$k=$v"
      ;;
    ROBOKASSA_*)
      echo "$k=len:${#v}"
      ;;
  esac
done < "$ENV"

echo ""
echo "=== BILLING READY ==="
docker exec docker-billing-1 wget -qO- http://127.0.0.1:8002/health 2>/dev/null || \
  docker exec docker-billing-1 wget -qO- http://127.0.0.1:8002/ready 2>/dev/null || \
  curl -s http://127.0.0.1:8002/health 2>/dev/null || echo "no health endpoint reachable"

echo ""
echo "=== BILLING LOGS (full last 200) ==="
docker logs docker-billing-1 --tail 200 2>&1

echo ""
echo "=== GATEWAY last 100 (pay/robo) ==="
docker logs docker-gateway-1 --tail 200 2>&1 | grep -iE 'robo|pay|topup|/billing|callback|result|400|500|401' | tail -50

echo ""
echo "=== RECENT ROBOKASSA INVOICES IN DB ==="
# try get POSTGRES from env
set -a
# shellcheck disable=SC1090
source "$ENV" 2>/dev/null || true
set +a
if command -v docker >/dev/null; then
  docker exec docker-billing-1 sh -c 'echo skip' >/dev/null 2>&1 || true
fi
PSQL_HOST="${POSTGRES_HOST:-108.174.78.39}"
PSQL_DB="${POSTGRES_DB:-vps_platform}"
PSQL_USER="${POSTGRES_USER:-vps}"
export PGPASSWORD="${POSTGRES_PASSWORD:-}"
if command -v psql >/dev/null && [ -n "${PGPASSWORD}" ]; then
  psql -h "$PSQL_HOST" -U "$PSQL_USER" -d "$PSQL_DB" -c \
    "SELECT id, provider, status, amount, robokassa_inv_id, LEFT(COALESCE(payment_url,''),60) url, created_at, updated_at
     FROM billing.invoices
     WHERE provider='robokassa' OR description ILIKE '%robo%'
     ORDER BY created_at DESC LIMIT 15;" 2>&1 || true
  psql -h "$PSQL_HOST" -U "$PSQL_USER" -d "$PSQL_DB" -c \
    "SELECT id, provider, status, amount, created_at
     FROM billing.invoices
     ORDER BY created_at DESC LIMIT 15;" 2>&1 || true
else
  echo "psql not available locally; try via container"
  # use postgres DSN from compose network if possible
fi

echo ""
echo "=== TRAEFIK/FRONT proxies if any ==="
docker logs docker-gateway-1 --since 24h 2>&1 | grep -iE 'robo|/payments|/billing/topup|OutSum|SignatureValue' | tail -30 || true
