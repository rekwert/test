#!/usr/bin/env bash
set -euo pipefail
BASE="${SMOKE_BASE:-http://127.0.0.1:8080/api/v1}"
PUBLIC="${SMOKE_PUBLIC:-https://cloud-hustle.com/api/v1}"
EMAIL="pub-smoke-$(date +%s)@example.com"
PW='SmokeTest1!'

echo "=== register+login chain email=$EMAIL ==="
reg=$(curl -s -D /tmp/rh -o /tmp/rb -w '%{http_code}' -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\",\"locale\":\"ru\"}")
echo "register: HTTP $reg"
grep -qi 'vps_refresh' /tmp/rh && echo "register cookie: OK" || echo "register cookie: MISSING"

loc=$(curl -s -D /tmp/lh -o /tmp/lb -w '%{http_code}' -X POST "$BASE/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}")
echo "login local: HTTP $loc"
grep -qi 'vps_refresh' /tmp/lh && echo "login local cookie: OK" || echo "login local cookie: MISSING"
python3 -c "import json;d=json.load(open('/tmp/lb'));print('login local token:', bool(d.get('access_token')))"

pub=$(curl -s -c /tmp/jar -b /tmp/jar -D /tmp/ph -o /tmp/pb -w '%{http_code}' -X POST "$PUBLIC/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}")
echo "login public (Traefik): HTTP $pub"
grep -qi 'vps_refresh' /tmp/ph && echo "login public cookie header: OK" || echo "login public cookie header: MISSING"
python3 -c "import json;d=json.load(open('/tmp/pb'));print('login public token:', bool(d.get('access_token')))"

ref=$(curl -s -c /tmp/jar -b /tmp/jar -D /tmp/frh -o /tmp/frb -w '%{http_code}' -X POST "$PUBLIC/auth/refresh")
echo "refresh public with cookie jar: HTTP $ref"
python3 -c "import json;d=json.load(open('/tmp/frb'));print('refresh token:', bool(d.get('access_token')))"

ACCESS=$(python3 -c "import json;print(json.load(open('/tmp/pb')).get('access_token',''))")
me=$(curl -s -o /tmp/me -w '%{http_code}' "$PUBLIC/auth/me" -H "Authorization: Bearer $ACCESS")
echo "auth/me public: HTTP $me"

fw=$(curl -s -o /tmp/fw -w '%{http_code}' "$PUBLIC/free-week" -H "Authorization: Bearer $ACCESS")
echo "free-week public: HTTP $fw body=$(tr -d '\n' < /tmp/fw | head -c 100)"

echo "=== DONE ==="
