#!/usr/bin/env python3
import json
import os
import urllib.parse
import urllib.request
import urllib.error


for raw in open("/opt/testVPStrade/infra/docker/.env", encoding="utf-8", errors="ignore"):
    raw = raw.strip()
    if raw and not raw.startswith("#") and "=" in raw:
        key, value = raw.split("=", 1)
        os.environ.setdefault(key, value.strip().strip('"').strip("'"))

base = os.environ.get("HOSTKEY_INVAPI_URL", "https://invapi.hostkey.com").rstrip("/")


def post(module, values):
    req = urllib.request.Request(
        f"{base}/{module}",
        data=urllib.parse.urlencode(values, doseq=True).encode(),
        headers={"Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=45) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            body = json.loads(raw)
        except json.JSONDecodeError:
            body = {"error": raw[:500]}
        return exc.code, body


status, login = post("auth.php", {"action": "login", "key": os.environ["HOSTKEY_API_TOKEN"]})
auth = login.get("result") if isinstance(login.get("result"), dict) else login
token = auth["token"]

status, result = post(
    "net.php",
    {
        "action": "add_ipv4",
        "token": token,
        "id": 53609,
        "port": "eth1",
        "ips[]": ["212.102.227.40"],
    },
)
for key in list(result):
    if any(term in key.lower() for term in ("token", "key", "password", "secret")):
        result[key] = "<redacted>"
print(json.dumps({"http": status, "response": result}, ensure_ascii=False, indent=2))
if status >= 400 or str(result.get("result", "")).lower() not in ("ok", "1"):
    raise SystemExit(1)
