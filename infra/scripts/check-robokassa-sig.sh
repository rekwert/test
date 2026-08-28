#!/bin/bash
set -a
source /opt/testVPStrade/infra/docker/.env
set +a

echo "=== BILLING LOGS after simulated bad/good webhook ==="
# simulate with wrong sig
docker exec docker-gateway-1 wget -qO- --post-data='OutSum=50.00&InvId=100025&SignatureValue=aabb' \
  http://127.0.0.1:8080/api/v1/webhooks/robokassa 2>&1 || true
sleep 1
docker logs docker-billing-1 --tail 20 2>&1

echo ""
echo "=== Any OK responses historically? ==="
docker run --rm --network docker_default postgres:16-alpine \
  psql "$POSTGRES_DSN" -c "SELECT status, COUNT(*) FROM billing.invoices WHERE provider='robokassa' GROUP BY status;"

echo ""
echo "=== Sample SignatureValue check (Password1 MD5) ==="
# OutSum 50.00 InvId 100025 MerchantLogin cloud-hustle
# Signature in URL: 5021308ce66ad161bc0c9e14d6cec5d6
PASS1="$ROBOKASSA_PASSWORD1"
SIG=$(echo -n "cloud-hustle:50.00:100025:${PASS1}" | md5sum | awk '{print $1}')
echo "expected_sig=$SIG"
echo "url_sig=5021308ce66ad161bc0c9e14d6cec5d6"
if [ "$SIG" = "5021308ce66ad161bc0c9e14d6cec5d6" ]; then echo "Password1 MATCHES payment URL signature"; else echo "Password1 MISMATCH vs payment URL (!)"; fi

echo ""
echo "=== Rate limit env ==="
grep -iE 'RATE|REDIS' /opt/testVPStrade/infra/docker/.env | sed 's/\(PASSWORD\|SECRET\|TOKEN\|KEY\)=.*/\1=***/'

echo ""
echo "=== When was billing last restarted, uptime ==="
docker inspect docker-billing-1 --format '{{.State.StartedAt}} {{.RestartCount}}'
