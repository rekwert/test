#!/usr/bin/env python3
import json
import os
import ssl
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
            print(f"{method} {path} HTTP {resp.status}")
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        print(f"{method} {path} HTTP {exc.code}: {raw[:1000]}")
        return exc.code, json.loads(raw) if raw else {}


_, templates = req("GET", "/media/templates/fromServerPackageSpec/1")
for group in templates.get("data") or []:
    if "ubuntu" in (group.get("name") or "").lower():
        print("UBUNTU GROUP:", json.dumps(group, indent=2)[:1500])

print(json.dumps(templates, indent=2)[:4000])

for path in [
    "/connectivity/ipblocks",
    "/connectivity/ipblocks/1",
    "/connectivity/ipblocks/1/ipv4?results=30",
]:
    code, body = req("GET", path)
    if isinstance(body, dict):
        print(path, "summary:", json.dumps(body, indent=2)[:2500])
