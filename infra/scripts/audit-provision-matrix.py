#!/usr/bin/env python3
"""Audit plan × region × OS provisioning readiness for the VPS portal."""
import json
import os
import re
import ssl
import subprocess
import sys
import urllib.error
import urllib.request

ENV_PATH = os.environ.get("ENV_FILE", "/opt/testVPStrade/infra/docker/.env")
GATEWAY = os.environ.get("GATEWAY_URL", "http://127.0.0.1:8080/api/v1")

# Catalog plan UUID → expected SolusVM plan id (NL ladder 6–9).
EXPECTED_PLAN_MAP = {
    "11111111-1111-1111-1111-111111111101": 6,  # PROSTO-1
    "11111111-1111-1111-1111-111111111102": 7,
    "11111111-1111-1111-1111-111111111103": 8,
    "11111111-1111-1111-1111-111111111104": 9,
    "11111111-1111-1111-1111-111111111211": 6,  # Midrange (same specs as PROSTO)
    "11111111-1111-1111-1111-111111111212": 7,
    "11111111-1111-1111-1111-111111111213": 8,
    "11111111-1111-1111-1111-111111111214": 9,
    # HUSTLE: closest NL plans until dedicated plans exist on SolusVM
    "11111111-1111-1111-1111-111111111221": 7,  # 1/2GB/20 → PROSTO-2-NL (2/2GB/40)
    "11111111-1111-1111-1111-111111111222": 8,
    "11111111-1111-1111-1111-111111111223": 9,
    "11111111-1111-1111-1111-111111111224": 9,  # 6/16GB — no exact plan; uses PROSTO-4-NL
}

SOLUS_PLAN_SPECS = {
    6: (1, 1024, 20),
    7: (2, 2048, 40),
    8: (4, 4096, 80),
    9: (6, 8192, 120),
}


def load_env(path):
    data = {}
    if not os.path.isfile(path):
        return data
    for line in open(path):
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        data[k.strip()] = v.strip()
    return data


def parse_plan_map(raw):
    out = {}
    for part in raw.split(","):
        part = part.strip()
        if not part or ":" not in part:
            continue
        k, v = part.split(":", 1)
        out[k.strip()] = int(v.strip())
    return out


def api_get(path):
    req = urllib.request.Request(GATEWAY + path, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())


def solus_get(path, token):
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    base = os.environ.get("SOLUSVM_API_URL", "https://66.248.206.14/api/v1").rstrip("/")
    req = urllib.request.Request(
        base + path,
        headers={"Authorization": f"Bearer {token}", "Accept": "application/json"},
    )
    with urllib.request.urlopen(req, context=ctx, timeout=60) as resp:
        return json.loads(resp.read())


def main():
    env = load_env(ENV_PATH)
    dsn = env.get("POSTGRES_DSN", "")
    plan_map = parse_plan_map(env.get("SOLUSVM_PLAN_MAP", ""))
    provision_regions = {r.strip() for r in env.get("SOLUSVM_PROVISION_REGIONS", "nl").split(",") if r.strip()}
    location_map = parse_plan_map(env.get("SOLUSVM_LOCATION_MAP", "").replace(":", ":"))
    cr_id = int(env.get("SOLUSVM_COMPUTE_RESOURCE_ID", "0") or "0")

    errors = []
    warnings = []

    print("=== Regions ===")
    regions = api_get("/catalog/regions").get("regions", [])
    for r in regions:
        flag = "OK" if r.get("available") else "SOON"
        prov = "provision" if r.get("code") in provision_regions else "no-node"
        print(f"  [{flag}] {r.get('code')} enabled={r.get('enabled')} available={r.get('available')} ({prov})")
        if r.get("available") and r.get("code") not in provision_regions:
            errors.append(f"region {r.get('code')} available in UI but missing from SOLUSVM_PROVISION_REGIONS")
        if r.get("code") in provision_regions and not r.get("available"):
            warnings.append(f"region {r.get('code')} in SOLUSVM_PROVISION_REGIONS but no capacity in DB")

    print("\n=== Plans × SolusVM mapping ===")
    plans = api_get("/plans").get("plans", [])
    for p in plans:
        pid = p["id"]
        mapped = plan_map.get(pid)
        expected = EXPECTED_PLAN_MAP.get(pid)
        name = p.get("name", pid)
        region = p.get("region", "nl")
        cpu, ram, disk = p.get("cpu"), p.get("ram_mb"), p.get("disk_gb")
        status = "OK"
        if mapped != expected:
            status = "MISMATCH"
            errors.append(f"plan {name}: env maps to SolusVM {mapped}, expected {expected}")
        elif mapped is None:
            status = "MISSING"
            errors.append(f"plan {name}: no SOLUSVM_PLAN_MAP entry")
        elif expected and expected in SOLUS_PLAN_SPECS:
            sc, sr, sd = SOLUS_PLAN_SPECS[expected]
            if (cpu, ram, disk) != (sc, sr, sd) and not name.upper().startswith("HUSTLE"):
                warnings.append(f"plan {name} catalog specs {cpu}/{ram}/{disk} differ from SolusVM plan {expected} {sc}/{sr}/{sd}")
            elif name.upper().startswith("HUSTLE"):
                warnings.append(f"plan {name}: HUSTLE uses closest SolusVM plan {expected} (specs may differ)")
        print(f"  [{status}] {name} region={region} -> solus plan {mapped} (catalog {cpu}vCPU {ram}MB {disk}GB)")

    print("\n=== OS templates ===")
    os_list = api_get("/catalog/os").get("os_templates", [])
    if dsn:
        sql = "SELECT id, external_version_id, active FROM vps.os_templates WHERE active ORDER BY sort_order;"
        out = subprocess.check_output(["psql", dsn, "-t", "-A", "-F", "|", "-c", sql], text=True)
        db_os = {}
        for line in out.strip().splitlines():
            oid, ext, active = line.split("|")
            db_os[oid] = int(ext) if ext.strip() else 0
        missing_os = []
        for t in os_list:
            oid = t["id"]
            ext = db_os.get(oid, 0)
            if ext <= 0:
                missing_os.append(oid)
                errors.append(f"os {oid}: no external_version_id in DB")
            else:
                print(f"  [OK] {oid} -> SolusVM os_image_version_id={ext}")
        if missing_os:
            print(f"  [FAIL] missing mapping: {', '.join(missing_os)}")
    else:
        print("  (skip DB check — POSTGRES_DSN not set)")
        for t in os_list[:5]:
            print(f"  [?] {t['id']}")

    print("\n=== SolusVM infrastructure ===")
    token_path = "/tmp/solus-api-token.json"
    if os.path.isfile(token_path):
        token = json.load(open(token_path))["access_token"]
        plans_resp = solus_get("/plans?per_page=50", token).get("data", [])
        solus_ids = {p["id"] for p in plans_resp}
        for pid, sid in EXPECTED_PLAN_MAP.items():
            if sid not in solus_ids:
                errors.append(f"SolusVM plan id {sid} not found on panel")
        if cr_id <= 0:
            errors.append("SOLUSVM_COMPUTE_RESOURCE_ID not set")
        else:
            print(f"  compute_resource_id={cr_id}")
        print(f"  visible SolusVM plans: {len(plans_resp)}")
    else:
        warnings.append("no /tmp/solus-api-token.json — skip SolusVM live checks")

    print("\n=== Summary ===")
    if errors:
        print(f"ERRORS ({len(errors)}):")
        for e in errors:
            print(f"  - {e}")
    if warnings:
        print(f"WARNINGS ({len(warnings)}):")
        for w in warnings:
            print(f"  - {w}")
    if not errors and not warnings:
        print("All checks passed.")
    elif not errors:
        print("No blocking errors (warnings only).")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
