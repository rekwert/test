#!/bin/bash
set -euo pipefail
ENV_FILE="${1:-/opt/testVPStrade/infra/docker/.env}"
TOKEN="${HOSTKEY_API_TOKEN:?set HOSTKEY_API_TOKEN}"

upsert() {
  local key="$1" val="$2"
  if grep -q "^${key}=" "$ENV_FILE"; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
  else
    printf '\n%s=%s\n' "$key" "$val" >> "$ENV_FILE"
  fi
}

upsert HOSTKEY_ENABLED true
upsert HOSTKEY_API_TOKEN "$TOKEN"
upsert HOSTKEY_INVAPI_URL https://invapi.hostkey.ru
upsert HOSTKEY_MARKUP_PERCENT 10
upsert HOSTKEY_SYNC_INTERVAL 5m
upsert HOSTKEY_PRICE_SLACK_PERCENT 5
upsert HOSTKEY_EXTRA_IPV4_MAX 4

grep '^HOSTKEY_' "$ENV_FILE" | sed 's/TOKEN=.*/TOKEN=***/'
