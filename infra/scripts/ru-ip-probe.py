#!/usr/bin/env python3
"""Run IP block probe (SSH 22, optional custom SSH, then RDP) from this host. Prints JSON."""

from __future__ import annotations

import json
import os
import socket
import sys
import time


TIMEOUT = 4.0


def probe_ssh(ip: str, port: int) -> tuple[bool, int, str]:
    start = time.time()
    try:
        sock = socket.create_connection((ip, port), TIMEOUT)
    except OSError:
        return False, int((time.time() - start) * 1000), "TCP недоступен"
    try:
        sock.settimeout(3.0)
        banner = sock.recv(512)
    except OSError:
        return False, int((time.time() - start) * 1000), "SSH banner не получен"
    finally:
        sock.close()
    text = banner.decode("utf-8", "replace").strip()
    if text.startswith("SSH-"):
        return True, int((time.time() - start) * 1000), "SSH version exchange успешен"
    return False, int((time.time() - start) * 1000), "Некорректный SSH banner"


def probe_rdp(ip: str) -> tuple[bool, int, str]:
    start = time.time()
    req = bytes(
        [
            0x03,
            0x00,
            0x00,
            0x13,
            0x0E,
            0xE0,
            0x00,
            0x00,
            0x00,
            0x00,
            0x00,
            0x01,
            0x00,
            0x08,
            0x00,
            0x00,
            0x00,
            0x00,
            0x00,
        ]
    )
    try:
        sock = socket.create_connection((ip, 3389), TIMEOUT)
    except OSError:
        return False, int((time.time() - start) * 1000), "TCP недоступен"
    try:
        sock.settimeout(3.0)
        sock.sendall(req)
        resp = sock.recv(256)
    except OSError:
        return False, int((time.time() - start) * 1000), "RDP ответ не получен"
    finally:
        sock.close()
    if len(resp) >= 7 and resp[0] == 0x03 and resp[5] == 0xD0:
        return True, int((time.time() - start) * 1000), "RDP handshake успешен"
    return False, int((time.time() - start) * 1000), "RDP handshake отклонён"


def run(ip: str, ssh_port: int = 0) -> dict:
    started = time.time()
    attempts: list[dict] = []
    n = 0

    ssh22_ok, ssh22_ms, ssh22_detail = probe_ssh(ip, 22)
    n += 1
    attempts.append(
        {
            "n": n,
            "ok": ssh22_ok,
            "protocol": "SSH · 22",
            "port": 22,
            "detail": ssh22_detail,
            "duration_ms": ssh22_ms,
        }
    )

    custom_ok = False
    custom_ms = 0
    custom_detail = ""
    if ssh_port > 0 and ssh_port != 22:
        custom_ok, custom_ms, custom_detail = probe_ssh(ip, ssh_port)
        n += 1
        attempts.append(
            {
                "n": n,
                "ok": custom_ok,
                "protocol": f"SSH · {ssh_port}",
                "port": ssh_port,
                "detail": custom_detail,
                "duration_ms": custom_ms,
            }
        )

    ssh_ok = ssh22_ok or custom_ok
    rdp_ok = False
    rdp_ms = 0
    rdp_detail = ""
    if not ssh_ok:
        rdp_ok, rdp_ms, rdp_detail = probe_rdp(ip)
        n += 1
        attempts.append(
            {
                "n": n,
                "ok": rdp_ok,
                "protocol": "RDP · 3389",
                "port": 3389,
                "detail": rdp_detail,
                "duration_ms": rdp_ms,
            }
        )

    ok = ssh_ok or rdp_ok
    if ssh22_ok:
        status = "pass"
        reason = "standard_ssh_passed"
        reason_text = "SSH на порту 22 доступен из РФ"
        protocol = "SSH · 22"
    elif custom_ok:
        status = "pass"
        reason = "custom_ssh_passed"
        reason_text = (
            f"SSH доступен из РФ на порту {ssh_port}. "
            "Порт 22 закрыт — это настройка firewall клиента, а не блокировка РКN"
        )
        protocol = f"SSH · {ssh_port}"
    elif rdp_ok:
        status = "pass"
        reason = "rdp_handshake_passed"
        reason_text = "RDP handshake пройден из РФ"
        protocol = "RDP · 3389"
    else:
        status = "fail"
        reason = "all_handshakes_failed"
        reason_text = "Handshake не пройден на 22/3389"
        if ssh_port > 0 and ssh_port != 22:
            reason_text += f" и на SSH {ssh_port}"
        reason_text += " — возможна блокировка РКN или закрыты порты"
        if ssh_port == 0:
            reason_text += ". Если SSH на нестандартном порту — укажите его в проверке"
        protocol = ""

    success = sum(1 for a in attempts if a["ok"])

    def port(ok: bool, ms: int, detail: str, skipped: bool) -> dict:
        out = {"reachable": ok, "handshake": ok, "skipped": skipped, "latency_ms": ms}
        if not ok and detail:
            out["error"] = detail
        return out

    result = {
        "ip": ip,
        "ok": ok,
        "status": status,
        "protocol": protocol,
        "duration_ms": int((time.time() - started) * 1000),
        "reason": reason,
        "reason_text": reason_text,
        "attempts_total": len(attempts),
        "attempts_success": success,
        "attempts": attempts,
        "ssh_22": port(ssh22_ok, ssh22_ms, ssh22_detail, False),
        "rdp_3389": port(rdp_ok, rdp_ms, rdp_detail, ssh_ok),
        "source": "ru",
    }
    if ssh_port > 0 and ssh_port != 22:
        result["ssh_custom"] = port(custom_ok, custom_ms, custom_detail, False)
        result["ssh_port_checked"] = ssh_port
    return result


def parse_ssh_port(raw) -> int:
    try:
        port = int(raw)
    except (TypeError, ValueError):
        return 0
    if port < 1 or port > 65535:
        return 0
    return port


def main() -> None:
    if len(sys.argv) == 2 and sys.argv[1] == "--serve":
        serve_http()
        return
    if len(sys.argv) not in (2, 3):
        print(json.dumps({"error": "usage: ru-ip-probe.py <ip> [ssh_port] | --serve"}))
        sys.exit(1)
    ip = sys.argv[1].strip()
    ssh_port = parse_ssh_port(sys.argv[2]) if len(sys.argv) == 3 else 0
    try:
        socket.inet_aton(ip)
    except OSError:
        print(json.dumps({"error": "invalid ip"}))
        sys.exit(1)
    print(json.dumps(run(ip, ssh_port), ensure_ascii=False))


def serve_http() -> None:
    from http.server import BaseHTTPRequestHandler, HTTPServer

    token = os.environ.get("RU_IP_PROBE_TOKEN", "").strip()
    port = int(os.environ.get("RU_IP_PROBE_PORT", "8787"))

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt: str, *args) -> None:
            return

        def do_POST(self) -> None:
            if self.path != "/probe":
                self.send_error(404)
                return
            if token:
                auth = self.headers.get("Authorization", "")
                if auth != f"Bearer {token}":
                    self.send_error(401)
                    return
            length = int(self.headers.get("Content-Length", "0") or "0")
            raw = self.rfile.read(length) if length > 0 else b"{}"
            try:
                payload = json.loads(raw.decode("utf-8"))
                ip = str(payload.get("ip", "")).strip()
                ssh_port = parse_ssh_port(payload.get("ssh_port", 0))
                socket.inet_aton(ip)
            except (json.JSONDecodeError, OSError, TypeError, ValueError):
                self.send_response(400)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"error": "valid ip required"}).encode())
                return
            body = json.dumps(run(ip, ssh_port), ensure_ascii=False).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)

    HTTPServer(("0.0.0.0", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
