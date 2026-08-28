#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/infra/docker"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE — copy from .env.mail.example" >&2
  exit 1
fi

echo "Starting Mail stack..."
docker compose -f docker-compose.mail.yml --env-file "$ENV_FILE" up -d

echo "Mail deploy complete."
