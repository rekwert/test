#!/usr/bin/env bash
set -euo pipefail
cd /opt/testVPStrade
echo "=== FRONT DEPLOY ==="
git fetch origin main
git reset --hard origin/main
echo "infra HEAD: $(git rev-parse --short HEAD)"
bash infra/scripts/deploy-front.sh
