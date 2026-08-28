#!/usr/bin/env python3
import json
import os
import ssl
import subprocess
import urllib.request

env_path = "/opt/testVPStrade/infra/docker/.env"
for line in open(env_path, encoding="utf-8"):
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    value = value.strip().strip('"').strip("'")
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
    data = json.loads(resp.read().decode())

print("=== VirtFusion Windows templates (package 1) ===")
for group in data.get("data", []):
    gname = group.get("name", "")
    for t in group.get("templates", []):
        label = (t.get("name", "") + " " + gname).lower()
        if "win" in label:
            print(f"  vf_id={t.get('id')} name={t.get('name')} version={t.get('version')}")

dsn = os.environ.get("POSTGRES_DSN", "")
if dsn:
    print("\n=== DB os_templates (windows) ===")
    subprocess.run(
        [
            "psql",
            dsn,
            "-c",
            "SELECT id, name, version, external_version_id, active "
            "FROM vps.os_templates WHERE id ILIKE 'windows%' ORDER BY id;",
        ],
        check=False,
    )
