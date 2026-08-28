#!/usr/bin/env bash
set -euo pipefail

USER="${GHCR_USER:-borishru-boop}"
TOKEN="${GHCR_TOKEN:-}"

if [[ -z "$TOKEN" ]]; then
  echo "Set GHCR_TOKEN (GitHub PAT with read:packages)" >&2
  exit 1
fi

echo "$TOKEN" | docker login ghcr.io -u "$USER" --password-stdin
echo "Logged in to ghcr.io as $USER"
