#!/usr/bin/env python3
import json
import os
import ssl
import subprocess
import urllib.request

env = {}
for line in open("/opt/testVPStrade/infra/docker/.env"):
    if line.startswith("POSTGRES_DSN="):
        dsn = line.split("=", 1)[1].strip()
        break
else:
    dsn = ""

if dsn:
    q = "SELECT name, region, status, capacity_instances, (SELECT count(*) FROM vps.instances i WHERE i.node_id = n.id AND i.state <> 'deleted') AS used FROM vps.nodes n;"
    print("nodes:")
    print(subprocess.check_output(["psql", dsn, "-c", q], text=True))
    q2 = "SELECT hostname, state, ip_address::text FROM vps.instances WHERE state <> 'deleted' ORDER BY created_at;"
    print("instances:")
    print(subprocess.check_output(["psql", dsn, "-c", q2], text=True))

token = json.load(open("/tmp/solus-api-token.json"))["access_token"]
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
req = urllib.request.Request(
    "https://66.248.206.14/api/v1/servers?per_page=20",
    headers={"Authorization": "Bearer " + token},
)
servers = json.loads(urllib.request.urlopen(req, context=ctx).read()).get("data", [])
print("solus servers:", len(servers))
for s in servers:
    print(" ", s.get("id"), s.get("name"), s.get("status"))
