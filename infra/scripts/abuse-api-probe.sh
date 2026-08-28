#!/usr/bin/env bash
set -euo pipefail
NET="${1:-docker_default}"
TOKEN="$(docker exec docker-vps-1 printenv ABUSE_INGEST_TOKEN)"
docker run --rm --network "$NET" curlimages/curl:8.5.0 -s -w "\nHTTP=%{http_code}\n" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Service-Token: bad-token" \
  -d '{"ip":"203.0.113.1","signal_type":"provider_complaint"}' \
  http://vps:8003/internal/abuse/signal || true
echo "---"
docker run --rm --network "$NET" curlimages/curl:8.5.0 -s -w "\nHTTP=%{http_code}\n" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Service-Token: ${TOKEN}" \
  -d '{"ip":"203.0.113.1","signal_type":"provider_complaint"}' \
  http://vps:8003/internal/abuse/signal
