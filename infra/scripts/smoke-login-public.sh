#!/usr/bin/env bash
set -euo pipefail
EMAIL="${1:-browser-smoke-1786462000@example.com}"
PW="${2:-SmokeTest1!}"
URL="${3:-https://cloud-hustle.com/api/v1/auth/login}"

echo "=== login public $URL ==="
code=$(curl -s -D /tmp/plh -o /tmp/plb -w '%{http_code}' -X POST "$URL" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}")
echo "HTTP $code"
grep -i 'set-cookie:.*vps_refresh' /tmp/plh && echo "cookie: vps_refresh OK" || echo "cookie: missing"
python3 - <<'PY'
import json
d=json.load(open("/tmp/plb"))
print("access_token:", "access_token" in d and bool(d.get("access_token")))
if d.get("error"):
    print("error:", d["error"])
PY

echo "=== login local gateway ==="
code2=$(curl -s -D /tmp/plh2 -o /tmp/plb2 -w '%{http_code}' -X POST 'http://127.0.0.1:8080/api/v1/auth/login' \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}")
echo "HTTP $code2"
grep -i 'set-cookie:.*vps_refresh' /tmp/plh2 && echo "cookie: vps_refresh OK" || echo "cookie: missing"

echo "=== admin redirect ==="
curl -sI https://cloud-hustle.com/vps/admin | grep -iE '^(HTTP|location:)'

echo "=== vnc viewer ==="
curl -s -o /dev/null -w 'vnc-viewer: %{http_code}\n' https://cloud-hustle.com/vps/vnc-viewer.html
