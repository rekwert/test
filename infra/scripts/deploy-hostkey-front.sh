#!/bin/bash
set -euo pipefail
FRONT_ROOT="${FRONT_ROOT:-/opt/FrontVPS}"
COMPOSE_DIR="${COMPOSE_DIR:-/opt/testVPStrade/infra/docker}"
BACKEND_ROOT="${BACKEND_ROOT:-/opt/testVPStrade}"
WEB_IMAGE=ghcr.io/borishru-boop/testvps-trade-web:latest

if [[ ! -d "$BACKEND_ROOT/.git" ]]; then
  echo "Backend repo not found at $BACKEND_ROOT" >&2
  exit 1
fi

echo "==> pull infra repo from $BACKEND_ROOT"
git -C "$BACKEND_ROOT" fetch origin main
git -C "$BACKEND_ROOT" reset --hard origin/main
echo "infra HEAD $(git -C "$BACKEND_ROOT" rev-parse --short HEAD)"

if [[ ! -d "$FRONT_ROOT/.git" ]]; then
  FRONT_ROOT=/opt/vps-portal
fi
if [[ ! -d "$FRONT_ROOT/.git" ]]; then
  echo "Front repo not found" >&2
  exit 1
fi

echo "==> pull frontend from $FRONT_ROOT"
git -C "$FRONT_ROOT" fetch origin main
git -C "$FRONT_ROOT" reset --hard origin/main
echo "HEAD $(git -C "$FRONT_ROOT" rev-parse --short HEAD)"

echo "==> build web image"
docker build -t "$WEB_IMAGE" "$FRONT_ROOT"

echo "==> restart web + traefik"
cd "$COMPOSE_DIR"
docker compose -f docker-compose.front.yml --env-file .env pull traefik
docker compose -f docker-compose.front.yml --env-file .env up -d web traefik

echo "Front deploy done."
