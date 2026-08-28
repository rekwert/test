#!/usr/bin/env python3
import json
import os
import urllib.parse
import urllib.request
import urllib.error


for raw in open("/opt/testVPStrade/infra/docker/.env", encoding="utf-8", errors="ignore"):
    raw = raw.strip()
    if raw and not raw.startswith("#") and "=" in raw:
        key, value = raw.split("=", 1)
        os.environ.setdefault(key, value.strip().strip('"').strip("'"))

base = os.environ.get("HOSTKEY_INVAPI_URL", "https://invapi.hostkey.com").rstrip("/")


def post(module, values):
    req = urllib.request.Request(
        f"{base}/{module}",
        data=urllib.parse.urlencode(values).encode(),
        headers={"Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=45) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            body = json.loads(raw)
        except json.JSONDecodeError:
            body = {"error": raw[:500]}
        return exc.code, body


_, login = post("auth.php", {"action": "login", "key": os.environ["HOSTKEY_API_TOKEN"]})
auth = login.get("result") if isinstance(login.get("result"), dict) else login
token = auth["token"]

message = """
Please attach the existing customer IPv4 subnet 212.102.227.0/24 (VLAN 701,
gateway 212.102.227.1) directly to the spare physical port eth1 of server
ID 53609 (main IP 66.151.40.165).

Requirements:
- Keep the current main connection on port 10G-2 / VLAN 704 unchanged.
- Keep server ID 17504 (185.84.224.84) and all its current IPs/networking unchanged.
- Do not route or forward traffic through server 17504.
- Both servers must have direct L2 access to VLAN 701 on separate physical ports.
  Server 17504 already uses VLAN 701 on eth0; server 53609 should use VLAN 701
  on its unused eth1 port.
- We allocate unique, non-overlapping addresses centrally in VirtFusion, so the
  same IPv4 will never be assigned to two virtual machines.

Test case: 212.102.227.40 is currently assigned to a VM on server 53609. The VM
is reachable from its hypervisor, but public traffic is still delivered to
185.84.224.84, which returns Destination Host Unreachable. InvAPI
net/add_ipv4 for server 53609 (ports 10G-2 and eth1) returns
"net/add_ipv4: invalid request".

Please enable VLAN 701 on server 53609 eth1 without disrupting either server.
""".strip()

status, result = post(
    "jira.php",
    {
        "action": "request_assistance",
        "token": token,
        "id": 53609,
        "terminate_reason_custom": message,
    },
)
for key in list(result):
    if any(term in key.lower() for term in ("token", "key", "password", "secret")):
        result[key] = "<redacted>"
print(json.dumps({"http": status, "response": result}, ensure_ascii=False, indent=2))
if status >= 400 or str(result.get("result", "")).lower() not in ("ok", "1"):
    raise SystemExit(1)
