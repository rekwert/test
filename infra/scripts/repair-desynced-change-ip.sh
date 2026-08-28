#!/usr/bin/env bash
# Repair instances left desynced after partial change-IP failures.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
cd "$ROOT/infra/docker"

repair() {
  local instance="$1" reach="$2" new_ip="$3" old_ip="$4" gw="$5"
  echo "=== repair $instance $reach -> $new_ip (release $old_ip) ==="
  docker compose -f docker-compose.back.yml --env-file "$ENV_FILE" run --rm --no-deps \
    --entrypoint /repair-change-ip vps \
    -instance "$instance" -reach-ip "$reach" -new-ip "$new_ip" -old-ip "$old_ip" -gateway "$gw" -prefix 24
}

# GB vps-sv9f8j: guest still on .6, portal/VF orphan .9
repair "7bf455ad-45a2-489a-ae13-bea4260c6844" "91.108.247.6" "91.108.247.9" "91.108.247.6" "91.108.247.1"

# DE vps-ac8owm: guest on .40, portal expects .39
repair "eda519e1-afdc-47cc-a9e7-502dbbcfc146" "212.102.227.40" "212.102.227.39" "212.102.227.40" "212.102.227.1"

echo "=== done ==="
