#!/usr/bin/env python3
import json
import os
import ssl
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

req = urllib.request.Request(
    base + "/media/templates/fromServerPackageSpec/1",
    headers={"Authorization": "Bearer " + key, "Accept": "application/json"},
)
with urllib.request.urlopen(req, context=ctx, timeout=60) as resp:
    data = json.loads(resp.read())

for group in data.get("data") or []:
    gname = group.get("name")
    for tmpl in group.get("templates") or []:
        print(f"{gname} | {tmpl.get('name')} | {tmpl.get('version')} | id={tmpl.get('id')}")
