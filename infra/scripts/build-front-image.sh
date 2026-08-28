#!/usr/bin/env bash
# Build portal web image from FrontVPS git (same Dockerfile as CI).
set -euo pipefail

export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
FRONT_ROOT="${FRONT_ROOT:-/opt/FrontVPS}"
if [[ -z "${IMAGE_TAG:-}" && -f "$ENV_FILE" ]]; then
  IMAGE_TAG="$(grep -m1 '^IMAGE_TAG=' "$ENV_FILE" | cut -d= -f2-)"
fi
TAG="${IMAGE_TAG:-latest}"
IMAGE="${WEB_IMAGE:-ghcr.io/borishru-boop/testvps-trade-web:$TAG}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE" >&2
  exit 1
fi

if [[ ! -d "$FRONT_ROOT/.git" ]]; then
  FRONT_ROOT=/opt/vps-portal
fi
if [[ ! -d "$FRONT_ROOT/.git" ]]; then
  echo "Front repo not found (set FRONT_ROOT=/path/to/FrontVPS)" >&2
  exit 1
fi

echo "Updating frontend from origin/main..."
git -C "$FRONT_ROOT" fetch origin main
git -C "$FRONT_ROOT" reset --hard origin/main
echo "Front HEAD: $(git -C "$FRONT_ROOT" rev-parse --short HEAD)"

API_URL="$(grep -m1 '^NEXT_PUBLIC_API_URL=' "$ENV_FILE" | cut -d= -f2-)"
API_URL="${API_URL:-https://cloud-hustle.com/api/v1}"

NO_CACHE="${NO_CACHE:-0}"
BUILD_FLAGS=()
if [[ "$NO_CACHE" == "1" ]]; then
  BUILD_FLAGS+=(--no-cache)
fi

echo "==> build $IMAGE (NEXT_PUBLIC_API_URL=$API_URL)"
docker build "${BUILD_FLAGS[@]}" \
  --build-arg "NEXT_PUBLIC_API_URL=$API_URL" \
  -t "$IMAGE" \
  "$FRONT_ROOT"

echo "Web image built: $IMAGE"
