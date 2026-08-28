#!/usr/bin/env python3
import json
import os
import urllib.parse
import urllib.request


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
    with urllib.request.urlopen(req, timeout=45) as response:
        return json.load(response)


login = post("auth.php", {"action": "login", "key": os.environ["HOSTKEY_API_TOKEN"]})
auth = login.get("result") if isinstance(login.get("result"), dict) else login
token = auth["token"]


def call(action, **values):
    return post("net.php", {"action": action, "token": token, **values})


checks = [
    ("prosto_status", "get_status", {"id": 17504, "port": "eth0"}),
    ("mid_status", "get_status", {"id": 53609, "port": "10G-2"}),
    (
        "prosto_free_pool",
        "show_ipv4_free",
        {"id": 17504, "port": "eth0", "ip": "212.102.227.40", "show_all": 1},
    ),
    (
        "mid_free_pool",
        "show_ipv4_free",
        {"id": 53609, "port": "10G-2", "ip": "212.102.227.40", "show_all": 1},
    ),
]

for label, action, values in checks:
    print(f"=== {label} ===")
    try:
        print(json.dumps(call(action, **values), ensure_ascii=False, indent=2)[:30000])
    except Exception as exc:
        print(f"{type(exc).__name__}: {exc}")
