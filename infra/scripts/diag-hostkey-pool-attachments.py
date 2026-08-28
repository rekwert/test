#!/usr/bin/env python3
import json
import os
import urllib.parse
import urllib.request


def load_env(path):
    for raw in open(path, encoding="utf-8", errors="ignore"):
        raw = raw.strip()
        if not raw or raw.startswith("#") or "=" not in raw:
            continue
        key, value = raw.split("=", 1)
        os.environ.setdefault(key, value.strip().strip('"').strip("'"))


load_env("/opt/testVPStrade/infra/docker/.env")
base = os.environ.get("HOSTKEY_INVAPI_URL", "https://invapi.hostkey.com").rstrip("/")
api_key = os.environ["HOSTKEY_API_TOKEN"]


def post(module, values):
    req = urllib.request.Request(
        f"{base}/{module}",
        data=urllib.parse.urlencode(values, doseq=True).encode(),
        headers={"Accept": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=45) as response:
        return json.load(response)


login = post("auth.php", {"action": "login", "key": api_key})
auth = login.get("result") if isinstance(login.get("result"), dict) else login
session = auth["token"]


def call(module, action, **values):
    return post(module, {"action": action, "token": session, **values})


def redacted(value):
    if isinstance(value, dict):
        out = {}
        for key, item in value.items():
            low = key.lower()
            if any(term in low for term in ("token", "password", "passwd", "secret", "key")):
                out[key] = "<redacted>"
            else:
                out[key] = redacted(item)
        return out
    if isinstance(value, list):
        return [redacted(item) for item in value]
    return value


for label, module, action, params in [
    ("servers", "eq.php", "search", {}),
    ("pool_ip", "ip.php", "get_ip", {"ip": "212.102.227.40"}),
]:
    try:
        result = call(module, action, **params)
        print(f"=== {label} ===")
        print(json.dumps(redacted(result), ensure_ascii=False, indent=2)[:30000])
    except Exception as exc:
        print(f"=== {label} error ===")
        print(type(exc).__name__, str(exc))

print("=== server details ===")
server_ids = call("eq.php", "search").get("servers", [])
for server_id in server_ids:
    try:
        detail = call("eq.php", "show", id=server_id)
        print(json.dumps(redacted(detail), ensure_ascii=False)[:12000])
        try:
            network = call("net.php", "get_status", id=server_id)
            print("network=" + json.dumps(redacted(network), ensure_ascii=False)[:8000])
        except Exception as exc:
            print(f"network_error={type(exc).__name__}: {exc}")
    except Exception as exc:
        print(f"id={server_id} error={type(exc).__name__}: {exc}")
