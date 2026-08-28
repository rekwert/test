#!/usr/bin/env python3
"""Delete uncommissioned orphan VF servers not linked in vps.instances.external_id."""
import json
import os
import ssl
import subprocess
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
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as exc:
        return exc.code, json.loads(exc.read().decode() or "{}")


def linked_external_ids() -> set[str]:
    dsn = os.environ.get("POSTGRES_DSN", "")
    if not dsn:
        return set()
    result = subprocess.run(
        ["psql", dsn, "-At", "-c",
         "SELECT DISTINCT external_id FROM vps.instances "
         "WHERE external_id IS NOT NULL AND TRIM(external_id) <> ''"],
        capture_output=True,
        text=True,
        check=False,
    )
    return {line.strip() for line in result.stdout.splitlines() if line.strip()}


dry_run = "--apply" not in sys.argv
linked = linked_external_ids()
_, listing = req("GET", "/servers?results=100")
rows = listing.get("data") or []
targets = []
for row in rows:
    sid = str(row.get("id", "")).strip()
    if not sid:
        continue
    if sid in linked:
        continue
    if (row.get("name") or "").strip():
        continue
    targets.append(row)
print(f"linked={sorted(linked)} orphans={len(targets)} dry_run={dry_run}")
for row in targets:
    sid = row["id"]
    print(f"  id={sid} created={row.get('created')}")
    if not dry_run:
        code, body = req("DELETE", f"/servers/{sid}")
        print(f"    DELETE HTTP {code} {body}")
