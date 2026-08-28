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
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        return exc.code, json.loads(raw) if raw else {}


_, listing = req("GET", "/servers?results=50")
rows = listing.get("data") or []
print(f"total_servers={listing.get('total', len(rows))}")
for row in rows:
    sid = row.get("id")
    _, detail = req("GET", f"/servers/{sid}")
    d = detail.get("data") or {}
    ip = ""
    net = d.get("network") or {}
    for iface in net.get("interfaces") or []:
        if (iface.get("type") or "").lower() == "public":
            v4 = iface.get("ipv4") or []
            if v4:
                ip = (v4[0] or {}).get("address") or ""
    print(
        f"id={sid} name={d.get('name')!r} hostname={d.get('hostname')!r} "
        f"state={d.get('state')} buildFailed={d.get('buildFailed')} "
        f"osTemplateId={(d.get('os') or {}).get('templateId')} ip={ip or '-'} "
        f"created={row.get('created')}"
    )
