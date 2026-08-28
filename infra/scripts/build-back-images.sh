#!/usr/bin/env bash
# Build all Back stack images from the current git tree (same contexts as CI).
set -euo pipefail

export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TAG="${IMAGE_TAG:-latest}"
PREFIX="${IMAGE_PREFIX:-ghcr.io/borishru-boop/testvps-trade}"
NO_CACHE="${NO_CACHE:-0}"
BUILD_FLAGS=()
if [[ "$NO_CACHE" == "1" ]]; then
  BUILD_FLAGS+=(--no-cache)
fi

build_image() {
  local name=$1
  local context=$2
  local dockerfile=$3
  local image="$PREFIX-$name:$TAG"
  echo "==> build $image"
  docker build "${BUILD_FLAGS[@]}" -t "$image" -f "$ROOT/$dockerfile" "$ROOT/$context"
}

cd "$ROOT"
HEAD="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "Building Back images from HEAD=$HEAD TAG=$TAG"

build_image gateway . apps/gateway/Dockerfile
build_image auth . services/auth/Dockerfile
build_image billing . services/billing/Dockerfile
build_image vps . services/vps/Dockerfile
build_image notification . services/notification/Dockerfile
build_image support . services/support/Dockerfile
build_image telegram-bot . services/telegram-bot/Dockerfile

echo "Back images built: $PREFIX-{gateway,auth,billing,vps,notification,support,telegram-bot}:$TAG"
