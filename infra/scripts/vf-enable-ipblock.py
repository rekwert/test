#!/usr/bin/env python3
import json
import os
import ssl
import sys
import urllib.error
import urllib.request

env_path = "/opt/testVPStrade/infra/docker/.env"
for line in open(env_path, encoding="utf-8"):
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        value = value[1:-1]
    os.environ[key] = value

base = os.environ["VIRTFUSION_API_URL"].rstrip("/")
key = os.environ["VIRTFUSION_API_KEY"]
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE


def req(method, path, body=None):
    headers = {"Authorization": "Bearer " + key, "Accept": "application/json"}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, context=ctx, timeout=60) as resp:
            raw = resp.read().decode()
            print(f"{method} {path} HTTP {resp.status}: {raw[:1000]}")
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        print(f"{method} {path} HTTP {exc.code}: {raw[:1000]}")
        return exc.code, json.loads(raw) if raw else {}


if len(sys.argv) > 1 and sys.argv[1] == "--enable-block":
    for method in ("PUT", "PATCH", "POST"):
        req(method, "/connectivity/ipblocks/1", {"enabled": True})

_, block = req("GET", "/connectivity/ipblocks/1")
print("enabled=", (block.get("data") or {}).get("enabled"))

for path in (
    "/compute/hypervisors/1",
    "/compute/hypervisors/1/network",
    "/compute/hypervisors/1/networks",
    "/compute/hypervisors/groups/1/resources",
):
    code, body = req("GET", path)
    if path.endswith("/resources") and isinstance(body, dict):
        data = (body.get("data") or [{}])[0]
        net = ((data.get("resources") or {}).get("network") or {}).get("total") or {}
        print("ipv4_free=", (net.get("ipv4") or {}).get("free"))
