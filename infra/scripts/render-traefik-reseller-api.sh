#!/usr/bin/env bash
# Render traefik/dynamic/reseller-api.yml for api.cloud-hustle.com subdomain.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/docker/.env}"
OUT="$ROOT/docker/traefik/dynamic/reseller-api.yml"
TPL="$ROOT/docker/traefik/dynamic/reseller-api.yml.tpl"

API_DOMAIN="${API_DOMAIN:-$(grep -m1 '^API_DOMAIN=' "$ENV_FILE" | cut -d= -f2-)}"
BACK_GATEWAY_URL="${BACK_GATEWAY_URL:-$(grep -m1 '^BACK_GATEWAY_URL=' "$ENV_FILE" | cut -d= -f2-)}"

if [[ ! -f "$TPL" ]]; then
  echo "missing $TPL" >&2
  exit 1
fi

if [[ -z "${API_DOMAIN:-}" ]]; then
  echo "skip reseller-api traefik (API_DOMAIN unset)" >&2
  exit 0
fi

TRANSPORT_BLOCK=""
if [[ "$BACK_GATEWAY_URL" == https://* ]]; then
  TRANSPORT_BLOCK="        serversTransport: internal-back-tls"
fi

export API_DOMAIN BACK_GATEWAY_URL TRANSPORT_BLOCK
envsubst '${API_DOMAIN} ${BACK_GATEWAY_URL} ${TRANSPORT_BLOCK}' < "$TPL" > "$OUT"
echo "Rendered $OUT (API_DOMAIN=$API_DOMAIN transport=$([[ -n "$TRANSPORT_BLOCK" ]] && echo tls || echo none))"