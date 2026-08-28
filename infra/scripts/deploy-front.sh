#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
BUILD_LOCAL="${BUILD_LOCAL:-1}"
FRONT_ROOT="${FRONT_ROOT:-/opt/FrontVPS}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE — copy from .env.front.example" >&2
  exit 1
fi

# vps_staff cookie is HMAC-signed with JWT_SECRET on the back host.
BACK_ENV="${BACK_ENV:-$ROOT/infra/docker/.env.back}"
if [[ -f "$BACK_ENV" ]]; then
  JWT_FROM_BACK="$(grep -m1 '^JWT_SECRET=' "$BACK_ENV" | cut -d= -f2- || true)"
  if [[ -n "${JWT_FROM_BACK:-}" ]]; then
    if grep -q '^STAFF_COOKIE_HMAC_KEY=' "$ENV_FILE"; then
      sed -i "s|^STAFF_COOKIE_HMAC_KEY=.*|STAFF_COOKIE_HMAC_KEY=${JWT_FROM_BACK}|" "$ENV_FILE"
    else
      echo "STAFF_COOKIE_HMAC_KEY=${JWT_FROM_BACK}" >> "$ENV_FILE"
    fi
  fi
fi

if [[ -d "$ROOT/.git" ]]; then
  echo "Updating infra repo from origin/main..."
  git -C "$ROOT" fetch origin
  git -C "$ROOT" reset --hard origin/main
  echo "infra HEAD: $(git -C "$ROOT" rev-parse --short HEAD)"
fi

cd "$ROOT/infra/docker"

mkdir -p traefik/dynamic
# Do not `source .env` here: TBANK_PASSWORD uses $$ which bash expands to PID.
export DOMAIN="${DOMAIN:-$(grep -m1 '^DOMAIN=' "$ENV_FILE" | cut -d= -f2-)}"
export BACK_GATEWAY_URL="${BACK_GATEWAY_URL:-$(grep -m1 '^BACK_GATEWAY_URL=' "$ENV_FILE" | cut -d= -f2-)}"
export MARKETING_DOMAIN="${MARKETING_DOMAIN:-$(grep -m1 '^MARKETING_DOMAIN=' "$ENV_FILE" | cut -d= -f2-)}"
if [[ -f traefik/dynamic/api.yml.tpl ]]; then
  bash "$ROOT/infra/scripts/render-traefik-api.sh"
fi
if [[ -f traefik/dynamic/reseller-api.yml.tpl ]]; then
  bash "$ROOT/infra/scripts/render-traefik-reseller-api.sh"
fi
if [[ -f traefik/dynamic/web.yml.tpl ]]; then
  envsubst '${DOMAIN}' < traefik/dynamic/web.yml.tpl > traefik/dynamic/web.yml
  echo "Rendered traefik/dynamic/web.yml"
fi
if [[ -f traefik/dynamic/marketing.yml.tpl ]]; then
  envsubst '${MARKETING_DOMAIN}' < traefik/dynamic/marketing.yml.tpl > traefik/dynamic/marketing.yml
  echo "Rendered traefik/dynamic/marketing.yml"
fi

if [[ "$BUILD_LOCAL" == "1" ]]; then
  echo "Building web image from FrontVPS (BUILD_LOCAL=1)..."
  FRONT_ROOT="$FRONT_ROOT" ENV_FILE="$ENV_FILE" bash "$ROOT/infra/scripts/build-front-image.sh"
else
  echo "Pulling web image from registry (BUILD_LOCAL=0, IMAGE_TAG=${IMAGE_TAG:-latest})..."
  docker compose -f docker-compose.front.yml --env-file "$ENV_FILE" pull web
fi

echo "Starting Front stack..."
docker compose -f docker-compose.front.yml --env-file "$ENV_FILE" up -d --force-recreate web traefik reseller-api-static

echo "Front deploy complete."
curl -sfI "https://${DOMAIN:-cloud-hustle.com}/vps/servers" | head -3 || true
