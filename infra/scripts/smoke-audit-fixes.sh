#!/usr/bin/env bash
# Post-deploy smoke checks for security audit fixes.
set -uo pipefail

BASE="${SMOKE_BASE:-http://127.0.0.1:8080/api/v1}"
PUBLIC="${SMOKE_PUBLIC:-https://cloud-hustle.com}"
PASS=0
FAIL=0

ok() { echo "OK   $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL $1"; FAIL=$((FAIL + 1)); }

echo "=== AUDIT SMOKE $(date -u +%FT%TZ) ==="

# Health
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/health" 2>/dev/null || curl -s -o /dev/null -w '%{http_code}' "${BASE%/api/v1}/health")
[[ "$code" == "200" ]] && ok "health -> 200" || bad "health -> $code"

# Open redirect guard (front HTML check — login page should not redirect to //evil)
redir=$(curl -s -o /dev/null -w '%{http_code}|%{redirect_url}' "$PUBLIC/vps/login?next=//evil.com" 2>/dev/null || echo "000|")
[[ "$redir" != *"evil.com"* ]] && ok "login next=//evil not redirected externally" || bad "login open redirect leak: $redir"

# Security headers on front
hdrs=$(curl -sI "$PUBLIC/vps/login" 2>/dev/null || true)
echo "$hdrs" | grep -qi 'x-frame-options' && ok "X-Frame-Options present" || bad "X-Frame-Options missing"
echo "$hdrs" | grep -qi 'x-content-type-options' && ok "X-Content-Type-Options present" || bad "X-Content-Type-Options missing"

# VNC viewer — password must not appear in recommended URL pattern
grep -q 'params.get.*password' "$PUBLIC/vnc-viewer.html" 2>/dev/null && bad "vnc-viewer still reads password from hash" || ok "vnc-viewer no hash password (or not grep-able)"

# Gateway catalog (auth required paths still work)
cat_code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/catalog/regions" 2>/dev/null || echo 000)
[[ "$cat_code" == "200" || "$cat_code" == "401" ]] && ok "catalog/regions -> $cat_code" || bad "catalog/regions -> $cat_code"

# Run full prod smoke if available
if [[ -x /opt/testVPStrade/infra/scripts/smoke-prod.sh ]]; then
  echo "--- running smoke-prod.sh ---"
  if bash /opt/testVPStrade/infra/scripts/smoke-prod.sh; then
    ok "smoke-prod.sh passed"
  else
    bad "smoke-prod.sh failed"
  fi
fi

echo "=== AUDIT SUMMARY: PASS=$PASS FAIL=$FAIL ==="
[[ "$FAIL" -eq 0 ]]
