#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade
echo "=== BEFORE ==="
git rev-parse --short HEAD
if ! grep -q '^VPS_FIELD_ENCRYPTION_KEY=' infra/docker/.env 2>/dev/null; then
  KEY="$(openssl rand -base64 32 | tr -d '\n')"
  echo "VPS_FIELD_ENCRYPTION_KEY=$KEY" >> infra/docker/.env
  echo "Added VPS_FIELD_ENCRYPTION_KEY to .env"
fi
git fetch origin
git reset --hard origin/main
echo "=== AFTER PULL ==="
git rev-parse --short HEAD
git log -1 --oneline
bash infra/scripts/deploy-back.sh
