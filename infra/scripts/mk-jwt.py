#!/usr/bin/env python3
import os, json, hmac, hashlib, base64, time, sys

secret = os.environ.get("JWT_SECRET")
if not secret:
    for line in open("/opt/testVPStrade/infra/docker/.env"):
        if line.startswith("JWT_SECRET="):
            secret = line.strip().split("=", 1)[1]
            break
if not secret:
    sys.exit("JWT_SECRET missing")

uid = sys.argv[1] if len(sys.argv) > 1 else "e160cfad-6859-41e4-951f-2cedc17b6479"

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
