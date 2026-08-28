#!/usr/bin/env bash
# Probe gateway proxy routes. HTTP 404 + body "404 page not found" = missing mount (stale gateway).
# 401/403/405/400/200/502 on upstream = route exists (502 = service down).
set -euo pipefail

BASE="${GATEWAY_URL:-http://127.0.0.1:8080}"
FAIL=0
OK=0
WARN=0

check() {
  local method=$1
  local path=$2
  local note=$3
  local code
  code=$(curl -s -o /tmp/audit-body.txt -w '%{http_code}' -X "$method" "$BASE$path" \
    -H 'Accept: application/json' 2>/dev/null || echo "000")
  local body
  body=$(head -c 120 /tmp/audit-body.txt 2>/dev/null | tr -d '\r')

  if [[ "$code" == "404" && "$body" == *"404 page not found"* ]]; then
    printf 'FAIL %-6s %-52s gateway 404 (route not mounted)\n' "$method" "$path"
    FAIL=$((FAIL + 1))
    return
  fi

  if [[ "$code" == "404" ]]; then
    printf 'WARN %-6s %-52s HTTP 404 upstream (%s)\n' "$method" "$path" "$note"
    WARN=$((WARN + 1))
    return
  fi

  printf 'OK   %-6s %-52s HTTP %s (%s)\n' "$method" "$path" "$code" "$note"
  OK=$((OK + 1))
}

echo "Gateway route audit: $BASE"
echo "HEAD $(git -C "$(dirname "$0")/../.." rev-parse --short HEAD 2>/dev/null || echo '?')"
echo "---"

check GET  /health                                              'gateway'
check GET  /api/v1/health                                       'gateway'
check GET  /api/v1/plans                                        'vps public'
check GET  /api/v1/catalog/regions                              'vps public'
check GET  /api/v1/free-week                                    'vps auth'
check GET  /api/v1/instances                                    'vps auth'
check GET  '/api/v1/instance-slug/vps-demo'                  'vps auth slug lookup'
check GET  /api/v1/orders                                       'vps POST-only'
check GET  /api/v1/admin/stats                                  'vps admin auth'
check GET  /api/v1/admin/nodes                                  'vps admin auth'
check GET  /api/v1/admin/instances                              'vps admin auth'
check GET  '/api/v1/admin/clients/00000000-0000-0000-0000-000000000000/instances' 'vps admin auth'
check GET  '/api/v1/admin/tools/vm-by-ip?ip=1.1.1.1'            'vps admin auth'
check POST '/api/v1/admin/abuse/cases/00000000-0000-0000-0000-000000000000/false-positive' 'vps admin auth'
check GET  /api/v1/auth/me                                      'auth'
check GET  /api/v1/referral                                     'auth'
check GET  /api/v1/billing/balance                              'billing auth'
check GET  /api/v1/billing/admin/stats/business                 'billing admin auth'
check GET  /api/v1/notifications                                'notification auth'
check GET  /api/v1/tickets                                      'support auth'
check GET  /api/v1/admin/tickets                                'support admin auth'
check GET  /api/v1/admin/shift                                  'support admin auth'
check GET  /api/v1/admin/workspace                              'support admin auth'
check GET  /api/v1/admin/queue/config                           'support admin auth'
check POST /api/v1/admin/notifications/send                     'notification admin auth'
check POST /api/v1/webhooks/heleket                             'billing webhook'

echo "---"
echo "OK=$OK WARN=$WARN FAIL=$FAIL"
if [[ "$FAIL" -gt 0 ]]; then
  echo "Some gateway routes are missing — rebuild: bash infra/scripts/build-back-images.sh" >&2
  exit 1
fi
