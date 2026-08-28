#!/usr/bin/env bash
# Append CUSTOM-1 plan UUIDs -> VF package 16 in infra/docker/.env (idempotent).
set -euo pipefail
ENV_FILE="${1:-/opt/testVPStrade/infra/docker/.env}"
PKG_ID="${2:-16}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing $ENV_FILE" >&2
  exit 1
fi
CUSTOM_ENTRIES=(
  "11111111-1111-1111-1111-111111111901:${PKG_ID}"
  "11111111-1111-1111-1111-111111111911:${PKG_ID}"
  "11111111-1111-1111-1111-111111111921:${PKG_ID}"
  "11111111-1111-1111-1111-111111111931:${PKG_ID}"
)
LINE=$(grep '^VIRTFUSION_PLAN_MAP=' "$ENV_FILE" || true)
if [[ -z "$LINE" ]]; then
  echo "VIRTFUSION_PLAN_MAP not found in $ENV_FILE" >&2
  exit 1
fi
MAP="${LINE#VIRTFUSION_PLAN_MAP=}"
CHANGED=0
for entry in "${CUSTOM_ENTRIES[@]}"; do
  uuid="${entry%%:*}"
  if echo "$MAP" | grep -q "$uuid:"; then
    echo "plan map already has $uuid"
  else
    MAP="${MAP},${entry}"
    CHANGED=1
    echo "plan map add $entry"
  fi
done
if [[ "$CHANGED" == "1" ]]; then
  sed -i "s|^VIRTFUSION_PLAN_MAP=.*|VIRTFUSION_PLAN_MAP=${MAP}|" "$ENV_FILE"
fi
echo "VF_CUSTOM_PLAN_MAP_OK"
