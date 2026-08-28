#!/bin/bash
set -euo pipefail
HOSTKEY_API_TOKEN="${HOSTKEY_API_TOKEN:?export HOSTKEY_API_TOKEN first}"
ROOT=/opt/testVPStrade
ENV_FILE="$ROOT/infra/docker/.env"
VPS_IMAGE=ghcr.io/borishru-boop/testvps-trade-vps:latest

echo "==> pull backend repo"
git -C "$ROOT" fetch origin main
git -C "$ROOT" reset --hard origin/main
echo "HEAD $(git -C "$ROOT" rev-parse --short HEAD)"

echo "==> build vps image"
docker build -t "$VPS_IMAGE" "$ROOT/services/vps"

echo "==> patch HOSTKEY env"
bash "$ROOT/infra/scripts/deploy-hostkey-env.sh" "$ENV_FILE"

echo "==> patch VirtFusion trial plan map"
bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ENV_FILE"

echo "==> migrations"
bash "$ROOT/infra/scripts/migrate.sh"

echo "==> restart vps stack"
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml --env-file "$ENV_FILE" up -d vps vps-worker

sleep 5
docker logs docker-vps-worker-1 2>&1 | grep -i hostkey | tail -5 || true
curl -sf http://127.0.0.1:8080/health && echo
