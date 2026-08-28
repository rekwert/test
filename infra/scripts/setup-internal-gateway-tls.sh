#!/usr/bin/env bash
# Self-signed TLS for internal gateway (Front VPS → Back VPS over HTTPS).
set -euo pipefail

TLS_DIR="${TLS_DIR:-/opt/internal-gateway-tls}"
# Selectel private network: back VPS is 192.168.0.2 (public 198.13.189.75)
BACK_VPS_IP="${BACK_VPS_IP:-192.168.0.2}"

mkdir -p "$TLS_DIR"
if [[ ! -f "$TLS_DIR/server.key" ]]; then
  openssl req -new -x509 -days 825 -nodes \
    -out "$TLS_DIR/server.crt" \
    -keyout "$TLS_DIR/server.key" \
    -subj "/CN=internal-gateway/O=CloudHustle" \
    -addext "subjectAltName=IP:${BACK_VPS_IP},DNS:internal-gateway"
  chmod 600 "$TLS_DIR/server.key"
fi

echo "Internal gateway TLS certs: $TLS_DIR"
echo "Set in back .env:"
echo "  GATEWAY_TLS_CERT=/certs/server.crt"
echo "  GATEWAY_TLS_KEY=/certs/server.key"
echo "  GATEWAY_TLS_PORT=8443"
echo "Set on front .env:"
echo "  BACK_GATEWAY_URL=https://${BACK_VPS_IP}:8443"
