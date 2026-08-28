#!/usr/bin/env bash
# Restrict Postgres/Redis to Back VPS only. Run on DB host after deploy-db.
set -euo pipefail

ENV_FILE="${ENV_FILE:-/opt/testVPStrade/infra/docker/.env}"
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
fi

BACK_VPS_IP="${BACK_VPS_IP:-}"
BACK_VPS_PUBLIC_IP="${BACK_VPS_PUBLIC_IP:-}"
if [[ -z "$BACK_VPS_IP" && -z "$BACK_VPS_PUBLIC_IP" ]]; then
  echo "Set BACK_VPS_IP and/or BACK_VPS_PUBLIC_IP in $ENV_FILE" >&2
  exit 1
fi

if ! command -v ufw >/dev/null 2>&1; then
  echo "ufw not installed" >&2
  exit 1
fi

allow_back() {
  local ip="$1"
  local label="$2"
  [[ -z "$ip" ]] && return 0
  ufw allow from "$ip" to any port 5432 proto tcp comment "Postgres $label"
  ufw allow from "$ip" to any port 6432 proto tcp comment "PgBouncer $label"
  ufw allow from "$ip" to any port 6379 proto tcp comment "Redis $label"
}

ufw --force enable
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
allow_back "$BACK_VPS_IP" "Back VPS private"
allow_back "$BACK_VPS_PUBLIC_IP" "Back VPS public"
ufw reload
echo "UFW hardened: Postgres/PgBouncer/Redis allowed from BACK_VPS_IP=${BACK_VPS_IP:-n/a} BACK_VPS_PUBLIC_IP=${BACK_VPS_PUBLIC_IP:-n/a}"
