#!/usr/bin/env python3
"""Audit VirtFusion edge firewall readiness (network mode + firewall API).

Usage (on Back with VF env):
  python3 vf-audit-firewall-edge.py
  python3 vf-audit-firewall-edge.py --json

Env: VIRTFUSION_API_URL, VIRTFUSION_API_KEY, POSTGRES_DSN (optional, for node map)
"""
from __future__ import annotations

import json
import os
import ssl
import sys
import urllib.error
import urllib.request
from typing import Any

# VirtFusion nf_type (from panel DB / docs): 0=direct/macvtap-like, 1=bridged, 2=routed, etc.
NF_TYPE_LABEL = {
    0: "direct/macvtap (firewall UNLIKELY)",
    1: "bridged (firewall OK)",
    2: "routed (firewall OK)",
    3: "nat",
    4: "openvswitch_bridged (VF docs: NO firewall / anti-hijack)",
    5: "isolated",
}


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def vf_request(method: str, path: str, body: dict | None = None) -> tuple[int, Any]:
    base = env("VIRTFUSION_API_URL").rstrip("/")
    if not base:
        raise SystemExit("VIRTFUSION_API_URL not set")
    url = base + path
    data = None
    headers = {
        "Authorization": f"Bearer {env('VIRTFUSION_API_KEY')}",
        "Accept": "application/json",
    }
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    ctx = ssl.create_default_context()
    if env("VIRTFUSION_INSECURE_TLS", "true").lower() in ("1", "true", "yes"):
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, context=ctx, timeout=30) as resp:
            raw = resp.read()
            if not raw:
                return resp.status, None
            return resp.status, json.loads(raw.decode())
    except urllib.error.HTTPError as e:
        raw = e.read().decode(errors="replace")
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {"raw": raw[:500]}
        return e.code, payload


def paginate(path: str) -> list[dict]:
    page = 1
    items: list[dict] = []
    while True:
        sep = "&" if "?" in path else "?"
        code, resp = vf_request("GET", f"{path}{sep}page={page}&results=100")
        if code != 200 or not isinstance(resp, dict):
            break
        batch = resp.get("data") or []
        if not isinstance(batch, list):
            break
        for row in batch:
            if isinstance(row, dict):
                items.append(row)
        last = resp.get("last_page") or 1
        if page >= int(last):
            break
        page += 1
    return items


def probe_rulesets() -> dict:
    out = {"endpoints_tried": [], "rulesets": []}
    for path in (
        "/firewall/rulesets",
        "/network/firewall/rulesets",
        "/settings/firewall/rulesets",
        "/firewall/rules",
        "/network/firewall/rules",
        "/settings/network/firewall/rulesets",
    ):
        code, resp = vf_request("GET", path)
        out["endpoints_tried"].append({"path": path, "status": code})
        if code == 200 and isinstance(resp, dict):
            data = resp.get("data")
            if isinstance(data, list) and data:
                out["rulesets"] = data
                out["rulesets_path"] = path
                break
    return out


def probe_server_firewall(server_id: str) -> dict:
    sid = str(server_id).strip()
    if not sid:
        return {"skipped": "no server id"}
    code, resp = vf_request("GET", f"/servers/{sid}/firewall/primary?sync=false")
    result = {"server_id": sid, "get_status": code, "get_body": resp}
    if code == 200:
        return result
    # enable probe only reports capability, does not persist if API rejects
    code2, resp2 = vf_request("POST", f"/servers/{sid}/firewall/primary/enable", {})
    result["enable_probe_status"] = code2
    result["enable_probe_body"] = resp2
    return result


def load_db_nodes() -> list[dict]:
    dsn = env("POSTGRES_DSN")
    if not dsn:
        return []
    try:
        import psycopg2  # type: ignore
    except ImportError:
        return []
    try:
        conn = psycopg2.connect(dsn)
        cur = conn.cursor()
        cur.execute(
            """
            SELECT n.name, n.region, n.external_id, n.status,
                   (SELECT COUNT(*)::int FROM vps.instances i
                    WHERE i.node_id = n.id AND i.state = 'running') AS running_vms
            FROM vps.nodes n
            WHERE n.external_id IS NOT NULL AND TRIM(n.external_id) <> ''
            ORDER BY n.region, n.name
            """
        )
        rows = []
        for name, region, ext, status, running in cur.fetchall():
            rows.append(
                {
                    "name": name,
                    "region": region,
                    "external_id": ext,
                    "status": status,
                    "running_vms": running,
                }
            )
        conn.close()
        return rows
    except Exception as e:
        return [{"error": str(e)}]


def main() -> int:
    report: dict[str, Any] = {
        "vf_api_url": env("VIRTFUSION_API_URL"),
        "hypervisors": [],
        "hypervisor_groups": [],
        "rulesets": {},
        "sample_servers": [],
        "db_nodes": load_db_nodes(),
        "verdict": {},
    }

    hvs = paginate("/compute/hypervisors")
    for hv in hvs:
        hv_id = hv.get("id")
        detail = {}
        if hv_id is not None:
            dcode, dresp = vf_request("GET", f"/compute/hypervisors/{hv_id}")
            if dcode == 200 and isinstance(dresp, dict):
                detail = dresp.get("data") if isinstance(dresp.get("data"), dict) else dresp
        nf = detail.get("nfType", detail.get("nf_type", hv.get("nfType", hv.get("nf_type"))))
        try:
            nf_int = int(nf) if nf is not None else None
        except (TypeError, ValueError):
            nf_int = None
        firewall_ok = nf_int in (1, 2)
        report["hypervisors"].append(
            {
                "id": hv_id,
                "name": hv.get("name"),
                "enabled": hv.get("enabled"),
                "commissioned": hv.get("commissioned"),
                "nf_type": nf_int,
                "nf_label": NF_TYPE_LABEL.get(nf_int, f"unknown({nf_int})"),
                "firewall_capable": firewall_ok,
                "ip": hv.get("ip"),
                "hostname": hv.get("hostname"),
            }
        )

    groups = paginate("/compute/hypervisors/groups")
    for g in groups:
        report["hypervisor_groups"].append(
            {"id": g.get("id"), "name": g.get("name"), "hypervisor_count": g.get("hypervisorCount")}
        )

    report["rulesets"] = probe_rulesets()

    servers = paginate("/servers")
    running = [
        s
        for s in servers
        if str(s.get("state", "")).lower() in ("complete", "running", "active", "ready")
        or str(s.get("status", "")).lower() in ("complete", "running", "active", "ready")
    ]
    sample = running[:3] if running else servers[:3]
    for s in sample:
        sid = s.get("id")
        fw = probe_server_firewall(str(sid) if sid is not None else "")
        report["sample_servers"].append(
            {
                "id": sid,
                "name": s.get("name"),
                "state": s.get("state", s.get("status")),
                "ip": s.get("ip"),
                "firewall": fw,
            }
        )

    capable = [h for h in report["hypervisors"] if h.get("firewall_capable")]
    not_capable = [h for h in report["hypervisors"] if not h.get("firewall_capable")]
    fw_api_ok = any(
        s.get("firewall", {}).get("get_status") == 200
        or s.get("firewall", {}).get("enable_probe_status") in (200, 201)
        for s in report["sample_servers"]
    )
    has_rulesets = bool(report["rulesets"].get("rulesets"))

    report["verdict"] = {
        "hypervisors_total": len(report["hypervisors"]),
        "firewall_capable_hvs": len(capable),
        "not_capable_hvs": len(not_capable),
        "firewall_api_responds": fw_api_ok,
        "rulesets_configured": has_rulesets,
        "can_integrate_edge": len(capable) > 0 and fw_api_ok,
        "blockers": [],
    }
    if not capable:
        report["verdict"]["blockers"].append(
            "No hypervisor with bridged/routed nf_type — edge firewall unavailable until network migration"
        )
    if not fw_api_ok:
        report["verdict"]["blockers"].append(
            "Firewall API did not respond OK on sample servers — check VF version/API token scope"
        )
    if not has_rulesets:
        report["verdict"]["blockers"].append(
            "No firewall rulesets found — create rulesets in VirtFusion panel before product integration"
        )

    if "--json" in sys.argv:
        print(json.dumps(report, indent=2, ensure_ascii=False))
    else:
        print("=== VirtFusion Edge Firewall Audit ===\n")
        print(f"API: {report['vf_api_url']}\n")
        print("Hypervisors:")
        for h in report["hypervisors"]:
            mark = "OK" if h.get("firewall_capable") else "NO"
            print(
                f"  [{mark}] id={h.get('id')} name={h.get('name')} "
                f"nf_type={h.get('nf_type')} ({h.get('nf_label')}) commissioned={h.get('commissioned')}"
            )
        print("\nDB nodes (our catalog):")
        for n in report["db_nodes"]:
            if "error" in n:
                print(f"  DB error: {n['error']}")
            else:
                print(
                    f"  {n.get('region')} {n.get('name')} ext={n.get('external_id')} "
                    f"status={n.get('status')} running_vms={n.get('running_vms')}"
                )
        print("\nFirewall rulesets:")
        rs = report["rulesets"]
        if rs.get("rulesets"):
            print(f"  Found {len(rs['rulesets'])} ruleset(s) via {rs.get('rulesets_path')}")
            for r in rs["rulesets"][:10]:
                print(f"    id={r.get('id')} name={r.get('name')}")
        else:
            print("  None found (tried:", ", ".join(p["path"] for p in rs.get("endpoints_tried", [])), ")")
        print("\nSample server firewall API:")
        for s in report["sample_servers"]:
            fw = s.get("firewall") or {}
            print(
                f"  server {s.get('id')} ({s.get('name')}): GET={fw.get('get_status')} "
                f"enable_probe={fw.get('enable_probe_status')}"
            )
        v = report["verdict"]
        print("\n=== VERDICT ===")
        print(f"  Can integrate VF edge: {'YES' if v.get('can_integrate_edge') else 'NO'}")
        for b in v.get("blockers") or []:
            print(f"  - {b}")

    return 0 if report["verdict"].get("can_integrate_edge") else 1


if __name__ == "__main__":
    raise SystemExit(main())
