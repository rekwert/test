#!/usr/bin/env bash
# Copy per-region HV SSH passwords from .env.probe into infra/docker/.env for vps smtpblock.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
PROBE_ENV="${PROBE_ENV:-$ROOT/infra/docker/.env.probe}"

[[ -f "$ENV_FILE" ]] || { echo "missing $ENV_FILE" >&2; exit 1; }
[[ -f "$PROBE_ENV" ]] || { echo "missing $PROBE_ENV" >&2; exit 1; }

# shellcheck disable=SC1090
source "$PROBE_ENV"

upsert_env() {
  local key="$1" val="$2"
  [[ -n "$val" ]] || return 0
  local tmp
  tmp="$(mktemp)"
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    grep -v "^${key}=" "$ENV_FILE" > "$tmp"
    printf '%s=%s\n' "$key" "$val" >> "$tmp"
    mv "$tmp" "$ENV_FILE"
  else
    printf '%s=%s\n' "$key" "$val" >> "$ENV_FILE"
  fi
}

for key in NL_SSH_PASS DE_SSH_PASS DE_MID_SSH_PASS FI_SSH_PASS GB_SSH_PASS; do
  upsert_env "$key" "${!key:-}"
done

echo "Synced HV SSH passwords into $ENV_FILE:"
grep -E '^(NL_SSH_PASS|DE_SSH_PASS|DE_MID_SSH_PASS|FI_SSH_PASS|GB_SSH_PASS)=' "$ENV_FILE" | cut -d= -f1
