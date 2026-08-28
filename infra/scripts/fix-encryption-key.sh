#!/usr/bin/env bash
set -euo pipefail
ENV=/opt/testVPStrade/infra/docker/.env
sed -i '/^VPS_FIELD_ENCRYPTION_KEY=/d' "$ENV"
KEY="$(openssl rand -base64 32 | tr -d '\n')"
echo "VPS_FIELD_ENCRYPTION_KEY=${KEY}" >> "$ENV"
echo "encryption key length: ${#KEY}"
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml --env-file .env up -d --force-recreate vps-worker vps
