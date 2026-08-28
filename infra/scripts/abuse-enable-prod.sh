#!/usr/bin/env bash
# Enable abuse auto-stop on Back production (env, migrations, rebuild vps images, smoke test).
# Does NOT git pull/push — safe for local patches before commit.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
COMPOSE_FILE="$ROOT/infra/docker/docker-compose.back.yml"

set_env() {
  local key="$1"
  local val="$2"
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
  else
    echo "${key}=${val}" >>"$ENV_FILE"
  fi
}

ensure_env_key() {
  local key="$1"
  local default="$2"
  if ! grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    echo "${key}=${default}" >>"$ENV_FILE"
    echo "Added ${key}=${default}"
  fi
}

[[ -f "$ENV_FILE" ]] || { echo "Missing $ENV_FILE" >&2; exit 1; }

if ! command -v psql >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y postgresql-client
fi

echo "=== abuse enable prod ==="

set_env ABUSE_ENABLED true
ensure_env_key ABUSE_SCAN_INTERVAL "2m"
ensure_env_key ABUSE_AUTO_STOP_THRESHOLD "100"
ensure_env_key ABUSE_TX_MBPS_THRESHOLD "150"
ensure_env_key ABUSE_TX_SUSTAINED_POLLS "4"
ensure_env_key ABUSE_SMTP_PROBE_INTERVAL "10m"
ensure_env_key ABUSE_MIN_DISTINCT_SIGNALS "2"
ensure_env_key ABUSE_NEW_INSTANCE_GRACE_BONUS "40"
ensure_env_key ABUSE_SIGNAL_WINDOW "1h"
ensure_env_key ABUSE_SIGNAL_DEDUPE "15m"

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

if [[ -z "${ABUSE_INGEST_TOKEN:-}" ]]; then
  NEW_TOKEN="$(openssl rand -hex 32)"
  set_env ABUSE_INGEST_TOKEN "$NEW_TOKEN"
  echo "Generated ABUSE_INGEST_TOKEN (${#NEW_TOKEN} chars) — save from .env for mail ingest scripts"
else
  echo "ABUSE_INGEST_TOKEN already set (${#ABUSE_INGEST_TOKEN} chars)"
fi

echo "Running migrations..."
bash "$ROOT/infra/scripts/migrate.sh"

echo "Building vps image from source..."
export BUILD_LOCAL=1
export IMAGE_TAG="${IMAGE_TAG:-$(grep -m1 '^IMAGE_TAG=' "$ENV_FILE" | cut -d= -f2-)}"
export IMAGE_TAG="${IMAGE_TAG:-local-abuse}"
PREFIX="${IMAGE_PREFIX:-ghcr.io/borishru-boop/testvps-trade}"
docker build -t "${PREFIX}-vps:${IMAGE_TAG}" -f "$ROOT/services/vps/Dockerfile" "$ROOT"
# Keep .env IMAGE_TAG aligned with the image we just built.
set_env IMAGE_TAG "$IMAGE_TAG"
echo "Built ${PREFIX}-vps:${IMAGE_TAG}"

echo "Recreating vps + vps-worker..."
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml --env-file "$ENV_FILE" up -d --force-recreate vps vps-worker

echo "Waiting for vps health..."
for i in $(seq 1 30); do
  if docker exec docker-vps-1 wget -qO- http://127.0.0.1:8003/health 2>/dev/null | grep -q ok; then
    break
  fi
  sleep 2
done

sed -i 's/\r$//' "$ROOT/infra/scripts/abuse-smoke-test.sh" 2>/dev/null || true
bash "$ROOT/infra/scripts/abuse-smoke-test.sh"
echo "=== abuse auto-stop enabled on prod ==="
