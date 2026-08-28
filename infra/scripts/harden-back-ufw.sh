#!/usr/bin/env bash
# Restrict back gateway :8080 to Front VPS only (UFW).
set -euo pipefail

ENV_FILE="${ENV_FILE:-/opt/testVPStrade/infra/docker/.env}"
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
fi

FRONT_IP="${FRONT_VPS_IP:-213.148.3.172}"
FRONT_PRIVATE_IP="${FRONT_PRIVATE_IP:-192.168.0.3}"
GATEWAY_PORT="${GATEWAY_PORT:-8080}"

if ! command -v ufw >/dev/null 2>&1; then
  echo "ufw not installed — install with: apt-get install -y ufw" >&2
  exit 1
fi

if ! ufw status | grep -qi active; then
  echo "Enabling UFW (allow SSH first)..."
  ufw allow OpenSSH
  ufw --force enable
fi

ufw allow from "$FRONT_PRIVATE_IP" to any port "$GATEWAY_PORT" proto tcp comment 'Front private -> Back gateway'
ufw allow from "$FRONT_IP" to any port "$GATEWAY_PORT" proto tcp comment 'Front public fallback -> Back gateway'
ufw status numbered | grep -E "$GATEWAY_PORT|$FRONT_IP|$FRONT_PRIVATE_IP" || true

if [[ -f "$ENV_FILE" ]]; then
  if grep -q '^BACK_GATEWAY_PUBLIC_OK=' "$ENV_FILE"; then
    sed -i 's|^BACK_GATEWAY_PUBLIC_OK=.*|BACK_GATEWAY_PUBLIC_OK=true|' "$ENV_FILE"
  else
    echo "BACK_GATEWAY_PUBLIC_OK=true" >>"$ENV_FILE"
  fi
  if grep -q '^TRUSTED_PROXY_IPS=' "$ENV_FILE"; then
    sed -i "s|^TRUSTED_PROXY_IPS=.*|TRUSTED_PROXY_IPS=${FRONT_PRIVATE_IP},${FRONT_IP}|" "$ENV_FILE"
  else
    echo "TRUSTED_PROXY_IPS=${FRONT_PRIVATE_IP},${FRONT_IP}" >>"$ENV_FILE"
  fi
  echo "Updated BACK_GATEWAY_PUBLIC_OK and TRUSTED_PROXY_IPS in $ENV_FILE"
fi

echo "HARDEN_BACK_UFW_DONE front_public=$FRONT_IP front_private=$FRONT_PRIVATE_IP port=$GATEWAY_PORT"
