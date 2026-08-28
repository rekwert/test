#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
BUILD_LOCAL="${BUILD_LOCAL:-1}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE — copy from .env.back.example" >&2
  exit 1
fi

if [[ -d "$ROOT/.git" ]]; then
  echo "Updating repo from origin/main..."
  git -C "$ROOT" fetch origin
  git -C "$ROOT" reset --hard origin/main
  echo "HEAD: $(git -C "$ROOT" rev-parse --short HEAD)"
fi

cd "$ROOT/infra/docker"

# Keep compose image tags aligned with local builds (BUILD_LOCAL=1).
export IMAGE_TAG="${IMAGE_TAG:-$(grep -m1 '^IMAGE_TAG=' "$ENV_FILE" | cut -d= -f2-)}"
export IMAGE_TAG="${IMAGE_TAG:-latest}"

echo "Running DB migrations..."
bash "$ROOT/infra/scripts/migrate.sh"

echo "Patching VirtFusion trial plan map..."
bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ENV_FILE"

if [[ "$BUILD_LOCAL" == "1" ]]; then
  echo "Building Back images from source (BUILD_LOCAL=1)..."
  bash "$ROOT/infra/scripts/build-back-images.sh"
else
  echo "Pulling Back images from registry (BUILD_LOCAL=0, IMAGE_TAG=${IMAGE_TAG:-latest})..."
  docker compose -f docker-compose.back.yml --env-file "$ENV_FILE" pull
fi

echo "Starting Back stack..."
docker compose -f docker-compose.back.yml --env-file "$ENV_FILE" up -d --force-recreate

echo "Auditing gateway routes..."
bash "$ROOT/infra/scripts/audit-gateway-routes.sh"

if [[ "${RUN_HARDEN_UFW:-1}" == "1" ]] && command -v ufw >/dev/null 2>&1; then
  echo "Hardening back firewall (UFW)..."
  bash "$ROOT/infra/scripts/harden-back-ufw.sh" || echo "WARN: UFW hardening skipped"
fi

echo "Back deploy complete."
curl -sf "http://127.0.0.1:${GATEWAY_PORT:-8080}/health" && echo
