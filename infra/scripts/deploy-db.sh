#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/infra/docker"

ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE — copy from .env.db.example" >&2
  exit 1
fi

echo "Starting DB stack..."
if [[ ! -f "${POSTGRES_SSL_DIR:-/opt/postgres-ssl}/server.key" ]]; then
  echo "Generating Postgres TLS certs..."
  POSTGRES_SSL_DIR="${POSTGRES_SSL_DIR:-/opt/postgres-ssl}" bash "$ROOT/infra/scripts/setup-postgres-ssl.sh" || true
fi

docker compose -f docker-compose.db.yml --env-file "$ENV_FILE" up -d

if [[ "${HARDEN_DB_UFW:-1}" == "1" ]] && command -v ufw >/dev/null 2>&1; then
  echo "Applying DB UFW rules..."
  ENV_FILE="$ENV_FILE" bash "$ROOT/infra/scripts/harden-db-ufw.sh" || echo "WARN: UFW harden failed (set BACK_VPS_IP in .env)" >&2
fi

echo "Waiting for postgres..."
for i in $(seq 1 30); do
  if docker compose -f docker-compose.db.yml --env-file "$ENV_FILE" exec -T postgres pg_isready -U "${POSTGRES_USER:-vps}" >/dev/null 2>&1; then
    echo "Postgres ready."
    exit 0
  fi
  sleep 2
done

echo "Postgres did not become ready in time" >&2
exit 1
