#!/usr/bin/env bash
# Render traefik/dynamic/api.yml — use serversTransport only for HTTPS backends.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/docker/.env}"
OUT="$ROOT/docker/traefik/dynamic/api.yml"
TPL="$ROOT/docker/traefik/dynamic/api.yml.tpl"

DOMAIN="${DOMAIN:-$(grep -m1 '^DOMAIN=' "$ENV_FILE" | cut -d= -f2-)}"
BACK_GATEWAY_URL="${BACK_GATEWAY_URL:-$(grep -m1 '^BACK_GATEWAY_URL=' "$ENV_FILE" | cut -d= -f2-)}"

if [[ ! -f "$TPL" ]]; then
  echo "missing $TPL" >&2
  exit 1
fi

TRANSPORT_BLOCK=""
if [[ "$BACK_GATEWAY_URL" == https://* ]]; then
  TRANSPORT_BLOCK="        serversTransport: internal-back-tls"
fi

export DOMAIN BACK_GATEWAY_URL TRANSPORT_BLOCK
envsubst '${DOMAIN} ${BACK_GATEWAY_URL} ${TRANSPORT_BLOCK}' < "$TPL" > "$OUT"
echo "Rendered $OUT (transport=$([[ -n "$TRANSPORT_BLOCK" ]] && echo tls || echo none))"
