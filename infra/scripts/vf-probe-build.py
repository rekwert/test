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
            print(f"{method} {path} HTTP {resp.status}: {raw[:800]}")
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        print(f"{method} {path} HTTP {exc.code}: {raw[:800]}")
        return exc.code, json.loads(raw) if raw else {}


print("=== templates ===")
req("GET", "/media/templates/fromServerPackageSpec/1")
print("=== hypervisors ===")
req("GET", "/compute/hypervisors")
print("=== group resources ===")
req("GET", "/compute/hypervisors/groups/1/resources")
print("=== servers ===")
req("GET", "/servers?results=20")

print("=== servers ===")
req("GET", "/servers?results=20")

create_test = "--create-test" in sys.argv
if not create_test:
    print("skip allocate/build (pass --create-test to create a probe VM)")
    sys.exit(0)

print("=== test allocate + build ===")
created_for_cleanup = False
code, created = req("POST", "/servers", {"packageId": 1, "userId": 1, "hypervisorId": 1})
server_id = None
if isinstance(created, dict):
    data = created.get("data") or created
    server_id = data.get("id")
    created_for_cleanup = code == 201 and server_id is not None
print("server_id:", server_id)

# Try build on first existing orphan if allocate failed
if not server_id:
    code, listing = req("GET", "/servers?results=5")
    if isinstance(listing, dict):
        rows = listing.get("data") or []
        if rows:
            server_id = rows[0].get("id")
            print("using existing server_id:", server_id)

if server_id:
    req("GET", f"/servers/{server_id}")
    code, templates = req("GET", "/media/templates/fromServerPackageSpec/1")
    os_id = None
    if isinstance(templates, dict):
        for group in templates.get("data") or []:
            for tmpl in group.get("templates") or []:
                name = (tmpl.get("name") or "").lower()
                version = (tmpl.get("version") or "").lower()
                if "ubuntu" in name and "22.04" in version:
                    os_id = tmpl.get("id")
                    break
            if os_id:
                break
    print("os_id:", os_id)
    if os_id:
        req(
            "POST",
            f"/servers/{server_id}/build",
            {
                "operatingSystemId": os_id,
                "name": "probe-test",
                "hostname": "probe-test",
                "vnc": False,
                "ipv6": False,
                "email": False,
            },
        )
        req("GET", f"/servers/{server_id}")

if created_for_cleanup and server_id:
    print("=== cleanup probe server ===")
    req("DELETE", f"/servers/{server_id}")
