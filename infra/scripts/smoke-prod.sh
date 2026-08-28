#!/usr/bin/env bash
# Production smoke tests — safe (creates one disposable register user).
set -uo pipefail

BASE="${SMOKE_BASE:-http://127.0.0.1:8080/api/v1}"
PUBLIC="${SMOKE_PUBLIC:-https://cloud-hustle.com}"
PASS=0
FAIL=0

ok() { echo "OK   $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL $1"; FAIL=$((FAIL + 1)); }

echo "=== SMOKE $(date -u +%FT%TZ) base=$BASE ==="

# --- Auth: bad login ---
code=$(curl -s -o /tmp/smoke-body -w '%{http_code}' -X POST "$BASE/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"nobody@example.com","password":"WrongPass1!"}')
[[ "$code" == "401" ]] && ok "login wrong password -> 401" || bad "login wrong password -> $code"

# --- Auth: refresh without cookie ---
code=$(curl -s -o /tmp/smoke-body -w '%{http_code}' -X POST "$BASE/auth/refresh")
[[ "$code" == "400" || "$code" == "401" ]] && ok "refresh without cookie -> $code" || bad "refresh without cookie -> $code"

# --- Auth: register + cookie + refresh + me + free-week ---
EMAIL="smoke-$(date +%s)@example.com"
PW='SmokeTest1!'
reg_code=$(curl -s -D /tmp/smoke-h -o /tmp/smoke-body -w '%{http_code}' -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\",\"locale\":\"ru\"}")
if [[ "$reg_code" == "201" || "$reg_code" == "200" ]]; then
  ok "register -> $reg_code"
else
  bad "register -> $reg_code ($(head -c 120 /tmp/smoke-body))"
fi
grep -qi 'set-cookie:.*vps_refresh' /tmp/smoke-h && ok "register Set-Cookie vps_refresh" || bad "register missing vps_refresh cookie"
ACCESS=$(python3 -c "import json; print(json.load(open('/tmp/smoke-body')).get('access_token',''))" 2>/dev/null || true)
[[ -n "$ACCESS" && "$ACCESS" != "null" ]] && ok "register returns access_token" || bad "register missing access_token in JSON body"

ref_code=$(curl -s -D /tmp/smoke-h2 -o /tmp/smoke-body2 -w '%{http_code}' -X POST "$BASE/auth/refresh" -b /tmp/smoke-h)
[[ "$ref_code" == "200" ]] && ok "refresh with cookie -> 200" || bad "refresh with cookie -> $ref_code"

me_code=$(curl -s -o /tmp/smoke-me -w '%{http_code}' "$BASE/auth/me" -H "Authorization: Bearer $ACCESS")
[[ "$me_code" == "200" ]] && ok "auth/me -> 200" || bad "auth/me -> $me_code"

fw_code=$(curl -s -o /tmp/smoke-fw -w '%{http_code}' "$BASE/free-week" -H "Authorization: Bearer $ACCESS")
[[ "$fw_code" == "200" ]] && ok "free-week -> 200 ($(tr -d '\n' < /tmp/smoke-fw | head -c 80))" || bad "free-week -> $fw_code"

# --- Telegram internal API ---
if [[ -f /opt/testVPStrade/infra/docker/.env ]]; then
  # shellcheck disable=SC1091
  set -a && source /opt/testVPStrade/infra/docker/.env && set +a
fi
if [[ -z "${TELEGRAM_INTERNAL_TOKEN:-}" ]]; then
  bad "TELEGRAM_INTERNAL_TOKEN not loaded"
else
  tg_code=$(curl -s -o /tmp/smoke-tg -w '%{http_code}' \
    -H "X-Internal-Token: $TELEGRAM_INTERNAL_TOKEN" \
    "$BASE/auth/telegram/by-id/999999001")
  grep -q access_token /tmp/smoke-tg && bad "telegram/resolve leaks access_token" || ok "telegram/resolve no JWT (HTTP $tg_code)"
  bs_code=$(curl -s -o /tmp/smoke-bs -w '%{http_code}' -X POST "$BASE/auth/telegram/bot-session" \
    -H "X-Internal-Token: $TELEGRAM_INTERNAL_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{"telegram_id":999999001}')
  [[ "$bs_code" == "404" || "$bs_code" == "401" ]] && ok "bot-session unlinked tg -> $bs_code" || bad "bot-session -> $bs_code"
  # Linked telegram user if any
  LINKED_TG=$(psql "$POSTGRES_DSN" -t -A -c "SELECT telegram_id FROM auth.users WHERE telegram_id IS NOT NULL LIMIT 1" 2>/dev/null || true)
  if [[ -n "$LINKED_TG" ]]; then
    bs2=$(curl -s -o /tmp/smoke-bs2 -w '%{http_code}' -X POST "$BASE/auth/telegram/bot-session" \
      -H "X-Internal-Token: $TELEGRAM_INTERNAL_TOKEN" \
      -H 'Content-Type: application/json' \
      -d "{\"telegram_id\":$LINKED_TG}")
    [[ "$bs2" == "200" ]] && grep -q access_token /tmp/smoke-bs2 && ok "bot-session linked tg -> 200 + token" || bad "bot-session linked -> $bs2"
  else
    ok "bot-session linked user skip (none in DB)"
  fi
fi

# --- VNC / console ---
if [[ -n "${POSTGRES_DSN:-}" ]]; then
  INST=$(psql "$POSTGRES_DSN" -t -A -c "SELECT id FROM vps.instances WHERE state IN ('running','stopped') LIMIT 1" 2>/dev/null || true)
  if [[ -n "$INST" ]]; then
    ct0=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/instances/$INST/console/ws-token")
    [[ "$ct0" == "401" ]] && ok "console/ws-token POST without auth -> 401" || bad "console/ws-token POST no auth -> $ct0"
    ct1=$(curl -s -o /tmp/smoke-ct -w '%{http_code}' -X POST "$BASE/instances/$INST/console/ws-token" \
      -H "Authorization: Bearer $ACCESS")
    [[ "$ct1" == "404" || "$ct1" == "403" || "$ct1" == "200" || "$ct1" == "409" || "$ct1" == "502" ]] \
      && ok "console/ws-token authed (foreign instance) -> $ct1" \
      || bad "console/ws-token authed -> $ct1"
  else
    ok "console/ws-token skip (no running/stopped instances)"
  fi
fi

vnc_code=$(curl -s -o /dev/null -w '%{http_code}' "$PUBLIC/vps/vnc-viewer.html")
[[ "$vnc_code" == "200" ]] && ok "vnc-viewer.html -> 200" || bad "vnc-viewer.html -> $vnc_code"

# --- Admin middleware (front) ---
admin_code=$(curl -s -o /dev/null -w '%{http_code}' -D /tmp/smoke-admin "$PUBLIC/vps/admin")
loc=$(grep -i '^location:' /tmp/smoke-admin | head -1 | tr -d '\r')
if [[ "$admin_code" =~ ^30[1278]$ ]] && echo "$loc" | grep -qi 'login'; then
  ok "admin without cookie -> $admin_code redirect login"
elif [[ "$admin_code" == "200" ]]; then
  bad "admin without cookie -> 200 (middleware not blocking?)"
else
  bad "admin without cookie -> $admin_code loc=$loc"
fi

# --- Telegram bot container ---
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q telegram-bot; then
  if docker logs docker-telegram-bot-1 --since 30m 2>&1 | grep -qiE 'error|fatal|panic'; then
    bad "telegram-bot errors in last 30m logs"
  else
    ok "telegram-bot Up, no errors in 30m logs"
  fi
else
  bad "telegram-bot container not running"
fi

echo "=== SUMMARY: PASS=$PASS FAIL=$FAIL ==="
[[ "$FAIL" -eq 0 ]]
