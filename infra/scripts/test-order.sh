#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env
export JWT_SECRET

TOKEN=$(python3 <<'PY'
import os, json, hmac, hashlib, base64, time
secret = os.environ["JWT_SECRET"]
uid = "e160cfad-6859-41e4-951f-2cedc17b6479"

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()

header = b64url(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
payload = b64url(json.dumps({
    "sub": uid,
    "email": "korosteliov.danil@mail.ru",
    "roles": ["client"],
    "exp": int(time.time()) + 3600,
}, separators=(",", ":")).encode())
msg = f"{header}.{payload}".encode()
sig = b64url(hmac.new(secret.encode(), msg, hashlib.sha256).digest())
print(f"{header}.{payload}.{sig}")
PY
)

curl -s -w "\nHTTP:%{http_code}\n" -X POST http://127.0.0.1:8080/api/v1/orders \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "plan_id": "11111111-1111-1111-1111-111111111101",
    "region": "nl",
    "hostname": "vps-test",
    "root_password": "KqyXBuHknwwxcTX7",
    "os_template_id": "ubuntu-24.04",
    "software_profile_id": "clean",
    "period_months": 1
  }'
