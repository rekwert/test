#!/usr/bin/env bash
# Recover MicroStack stuck on "Waiting for ...:5672" (common on 22.04 + public IP).
set -euo pipefail
[[ "$(id -u)" -eq 0 ]] || { echo "Run as root"; exit 1; }

echo "=== 1. Stop stuck init ==="
pkill -f microstack_init 2>/dev/null || true
sleep 2

echo "=== 2. Hostname / hosts (RabbitMQ binding) ==="
HOST="$(hostname -s)"
grep -q "$HOST" /etc/hosts || echo "127.0.1.1 $HOST" >> /etc/hosts
MAIN_IP="$(hostname -I | awk '{print $1}')"
echo "Main IP: $MAIN_IP"

echo "=== 3. Restart microstack snap ==="
snap restart microstack
echo "Waiting 90s for services..."
sleep 90
snap services microstack

echo "=== 4. RabbitMQ ports ==="
ss -lntp | grep -E '5672|25672' || echo "RabbitMQ still not listening"

echo "=== 5. Recent logs ==="
snap logs microstack -n 30 2>/dev/null | tail -30 || true

echo "=== 6. Retry init ==="
if ss -lntp | grep -q 5672; then
  microstack init --auto --control
else
  echo ""
  echo "RabbitMQ did not start. Full reset recommended:"
  echo "  snap remove microstack --purge"
  echo "  snap install microstack --beta --devmode"
  echo "  bash infra/openstack/control-install-microstack.sh"
  echo ""
  echo "OR reinstall OS as Ubuntu 24.04 and use Sunbeam (recommended long-term)."
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/bootstrap-dev.sh" | tee /root/openstack-bootstrap-retry.log
echo "Done. cat /root/openstack-portal-dev.env"
