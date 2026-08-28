#!/usr/bin/env bash
# 3x-ui VLESS Reality (www.5ka.ru) + standalone Hysteria2 (www.wikipedia.org, UDP). Prints BUNDLE_JSON: line.
set -euo pipefail

SERVER_IP="${SERVER_IP:-}"
VLESS_SNI="${VLESS_SNI:-www.5ka.ru}"
HY2_SNI="${HY2_SNI:-www.wikipedia.org}"
VLESS_PORT="${VLESS_PORT:-443}"
HY2_PORT="${HY2_PORT:-443}"
PANEL_PORT="${PANEL_PORT:-2053}"
PANEL_USER="${PANEL_USER:-admin}"
PANEL_PASS="$(openssl rand -base64 18 | tr -d '/+=' | head -c 16)"
VLESS_CLIENT_EMAIL="${VLESS_CLIENT_EMAIL:-vless-default}"
HY2_CLIENT_EMAIL="${HY2_CLIENT_EMAIL:-hy2-default}"

fail() {
  echo "bootstrap_error: $*" >&2
  exit 1
}

if [[ -z "$SERVER_IP" ]]; then
  SERVER_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
fi
[[ -n "$SERVER_IP" ]] || fail "SERVER_IP empty"

export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y
  apt-get install -y curl wget ca-certificates tar socat jq openssl python3 sqlite3
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y curl wget ca-certificates tar socat jq openssl python3
elif command -v yum >/dev/null 2>&1; then
  yum install -y curl wget ca-certificates tar socat jq openssl python3
fi

if [[ ! -x /usr/local/x-ui/x-ui && ! -x /usr/bin/x-ui ]]; then
  curl -fsSL https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh -o /tmp/3x-ui-install.sh
  chmod +x /tmp/3x-ui-install.sh
  printf '\n\n\n\n\n' | bash /tmp/3x-ui-install.sh >/tmp/3x-ui-install.log 2>&1 || fail "3x-ui installer failed"
fi

XUI_BIN="/usr/local/x-ui/x-ui"
if [[ ! -x "$XUI_BIN" ]]; then
  XUI_BIN="/usr/bin/x-ui"
fi
[[ -x "$XUI_BIN" ]] || fail "x-ui binary missing"

_xarch="amd64"
case "$(uname -m)" in
  x86_64 | amd64 | x64) _xarch="amd64" ;;
  aarch64 | arm64) _xarch="arm64" ;;
  armv7* | armv7) _xarch="armv7" ;;
esac
XRAY_BIN=""
for _candidate in \
  "/usr/local/x-ui/bin/xray-linux-${_xarch}" \
  "/usr/local/x-ui/bin/xray-linux-amd64" \
  "/usr/local/x-ui/bin/xray"; do
  if [[ -f "$_candidate" ]]; then
    chmod +x "$_candidate" 2>/dev/null || true
    XRAY_BIN="$_candidate"
    break
  fi
done
if [[ -z "$XRAY_BIN" ]]; then
  XRAY_BIN="$(find /usr/local/x-ui/bin -maxdepth 1 -name 'xray-linux-*' -type f 2>/dev/null | head -1)"
  [[ -n "$XRAY_BIN" ]] && chmod +x "$XRAY_BIN" 2>/dev/null || true
fi
[[ -n "$XRAY_BIN" && -x "$XRAY_BIN" ]] || fail "xray binary missing (checked bin/xray-linux-*)"

"$XUI_BIN" setting -username "$PANEL_USER" -password "$PANEL_PASS" -port "$PANEL_PORT" -webBasePath / >/dev/null 2>&1 \
  || "$XUI_BIN" setting -username "$PANEL_USER" -password "$PANEL_PASS" -port "$PANEL_PORT" >/dev/null

for _db in /etc/x-ui/x-ui.db /usr/local/x-ui/db/x-ui.db; do
  if [[ -f "$_db" ]] && command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$_db" "UPDATE setting SET value='/' WHERE key='webBasePath';" 2>/dev/null || true
    sqlite3 "$_db" "UPDATE settings SET value='/' WHERE key='webBasePath';" 2>/dev/null || true
  fi
done

systemctl restart x-ui >/dev/null 2>&1 || systemctl restart x-ui.service

for _wait in $(seq 1 30); do
  _db_ok=0
  for _db in /etc/x-ui/x-ui.db /usr/local/x-ui/db/x-ui.db; do
    if [[ ! -f "$_db" ]]; then
      continue
    fi
    if command -v sqlite3 >/dev/null 2>&1; then
      for _tbl in setting settings; do
        if sqlite3 "$_db" "SELECT 1 FROM ${_tbl} WHERE key='webBasePath' LIMIT 1;" 2>/dev/null | grep -q 1; then
          _db_ok=1
          break 2
        fi
      done
    fi
  done
  if [[ "$_db_ok" -eq 1 ]]; then
    break
  fi
  sleep 2
done

sleep 8

KEY_OUT="$("$XRAY_BIN" x25519 2>/dev/null || true)"
REALITY_PRIVATE="$(echo "$KEY_OUT" | awk -F': *' '/Private/ {print $2; exit}')"
REALITY_PUBLIC="$(echo "$KEY_OUT" | awk -F': *' '/Public/ {print $2; exit}')"
if [[ -z "$REALITY_PUBLIC" ]]; then
  REALITY_PRIVATE="$(echo "$KEY_OUT" | awk -F': *' '/PrivateKey/ {print $2; exit}')"
  REALITY_PUBLIC="$(echo "$KEY_OUT" | awk -F': *' '/Password/ {print $2; exit}')"
fi
[[ -n "$REALITY_PRIVATE" && -n "$REALITY_PUBLIC" ]] || fail "x25519 keygen failed"

SHORT_ID="$(openssl rand -hex 4)"
CLIENT_UUID="$(cat /proc/sys/kernel/random/uuid)"
HY2_PASS="$(openssl rand -hex 12)"

CERT_DIR="/etc/hysteria"
mkdir -p "$CERT_DIR"
openssl req -x509 -nodes -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout "$CERT_DIR/key.pem" -out "$CERT_DIR/cert.pem" -days 3650 \
  -subj "/CN=${HY2_SNI}" -addext "subjectAltName=DNS:${HY2_SNI}" >/dev/null 2>&1

export SERVER_IP VLESS_SNI HY2_SNI VLESS_PORT HY2_PORT PANEL_PORT PANEL_USER PANEL_PASS
export CLIENT_UUID HY2_PASS REALITY_PRIVATE REALITY_PUBLIC SHORT_ID CERT_DIR VLESS_CLIENT_EMAIL HY2_CLIENT_EMAIL

python3 <<'PY'
import json, os, sqlite3, time, urllib.parse, urllib.request, http.cookiejar

def env(name, default=""):
    return os.environ.get(name, default)

def read_panel_config(default_port):
    for db_path in ("/etc/x-ui/x-ui.db", "/usr/local/x-ui/db/x-ui.db"):
        if not os.path.isfile(db_path):
            continue
        port = default_port
        base_path = "/"
        try:
            conn = sqlite3.connect(db_path)
            cur = conn.cursor()
            for table in ("setting", "settings"):
                try:
                    cur.execute(f"SELECT key, value FROM {table} WHERE key IN ('webPort', 'webBasePath')")
                    for key, value in cur.fetchall():
                        if key == "webPort" and str(value).strip().isdigit():
                            port = int(value)
                        elif key == "webBasePath" and value:
                            base_path = str(value).strip() or "/"
                except sqlite3.Error:
                    continue
            conn.close()
        except sqlite3.Error:
            continue
        if not base_path.startswith("/"):
            base_path = "/" + base_path
        if base_path != "/":
            base_path = base_path.rstrip("/") + "/"
        return port, base_path
    return default_port, "/"

def normalize_base(panel_base):
    normalized = (panel_base or "/").strip()
    if not normalized.startswith("/"):
        normalized = "/" + normalized
    if normalized != "/" and not normalized.endswith("/"):
        normalized += "/"
    return normalized

def panel_prefix(panel_base):
    base = normalize_base(panel_base)
    if base == "/":
        return "/"
    return base

def csrf_url(origin, panel_base):
    prefix = panel_prefix(panel_base)
    if prefix == "/":
        return origin + "/csrf-token"
    return origin + prefix.rstrip("/") + "/csrf-token"

def login_endpoint(origin, panel_base):
    prefix = panel_prefix(panel_base)
    if prefix == "/":
        return origin + "/login"
    return origin + prefix.rstrip("/") + "/login"

def api_path(panel_base, path):
    suffix = path if path.startswith("/") else "/" + path
    prefix = panel_prefix(panel_base)
    if prefix == "/":
        return suffix
    return prefix.rstrip("/") + suffix

def fetch_csrf_token(opener, csrf_endpoint):
    with opener.open(csrf_endpoint, timeout=10) as resp:
        body = resp.read().decode("utf-8", "replace")
    try:
        data = json.loads(body) if body.strip() else {}
    except json.JSONDecodeError as exc:
        raise RuntimeError("csrf token response invalid") from exc
    token = data.get("obj")
    if not token:
        raise RuntimeError("csrf token missing")
    return token


def cert_pin_sha256_hex(cert_path):
    import subprocess
    out = subprocess.check_output(
        ["openssl", "x509", "-noout", "-fingerprint", "-sha256", "-in", cert_path],
        text=True,
    ).strip()
    fp = out.rsplit("=", 1)[-1]
    return fp.replace(":", "").lower()
def wait_panel(opener, csrf_endpoint):
    for _ in range(120):
        try:
            fetch_csrf_token(opener, csrf_endpoint)
            return
        except Exception:
            time.sleep(1)
    raise RuntimeError("panel not ready")

def build_panel_public_url(server_ip, panel_port, panel_base):
    path = api_path(panel_base, "/panel/")
    if not path.endswith("/"):
        path += "/"
    return f"http://{server_ip}:{panel_port}{path}"

def login_panel(opener, csrf_endpoint, login_url, panel_user, panel_pass):
    token = fetch_csrf_token(opener, csrf_endpoint)
    payload = json.dumps({"username": panel_user, "password": panel_pass}).encode()
    req = urllib.request.Request(
        login_url,
        data=payload,
        headers={
            "Content-Type": "application/json",
            "Accept": "application/json",
            "X-CSRF-Token": token,
        },
        method="POST",
    )
    with opener.open(req, timeout=30) as resp:
        body = resp.read().decode("utf-8", "replace")
    try:
        data = json.loads(body) if body.strip() else {}
    except json.JSONDecodeError:
        data = {"raw": body}
    if data.get("success") is False:
        raise RuntimeError("panel login failed: " + str(data.get("msg") or data))
    return token

server_ip = env("SERVER_IP")
vless_sni = env("VLESS_SNI", "www.5ka.ru")
hy2_sni = env("HY2_SNI", "www.wikipedia.org")
vless_port = int(env("VLESS_PORT", "443"))
hy2_port = int(env("HY2_PORT", "443"))
panel_port = int(env("PANEL_PORT", "2053"))
panel_user = env("PANEL_USER", "admin")
panel_pass = env("PANEL_PASS")
client_uuid = env("CLIENT_UUID")
hy2_pass = env("HY2_PASS")
priv = env("REALITY_PRIVATE")
pub = env("REALITY_PUBLIC")
short_id = env("SHORT_ID")
cert_dir = env("CERT_DIR")
vless_client_email = env("VLESS_CLIENT_EMAIL", "vless-default")
hy2_client_email = env("HY2_CLIENT_EMAIL", "hy2-default")

panel_port, panel_base = read_panel_config(panel_port)
origins = []
for host in ("127.0.0.1", server_ip):
    if host and f"http://{host}:{panel_port}" not in origins:
        origins.append(f"http://{host}:{panel_port}")
origin = origins[0]
csrf_endpoint = csrf_url(origin, panel_base)
login_url = login_endpoint(origin, panel_base)
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
csrf_token = ""


def get(path):
    global csrf_token
    if not csrf_token:
        csrf_token = fetch_csrf_token(opener, csrf_endpoint)
    req = urllib.request.Request(
        origin + api_path(panel_base, path),
        headers={
            "Accept": "application/json",
            "X-CSRF-Token": csrf_token,
        },
        method="GET",
    )
    with opener.open(req, timeout=60) as resp:
        body = resp.read().decode("utf-8", "replace")
    try:
        return json.loads(body) if body.strip() else {}
    except json.JSONDecodeError:
        return {"raw": body}
def post(path, payload):
    global csrf_token
    if not csrf_token:
        csrf_token = fetch_csrf_token(opener, csrf_endpoint)
    data = json.dumps(payload).encode()
    req = urllib.request.Request(
        origin + api_path(panel_base, path),
        data=data,
        headers={
            "Content-Type": "application/json",
            "Accept": "application/json",
            "X-CSRF-Token": csrf_token,
        },
        method="POST",
    )
    with opener.open(req, timeout=60) as resp:
        body = resp.read().decode("utf-8", "replace")
    try:
        return json.loads(body) if body.strip() else {}
    except json.JSONDecodeError:
        return {"raw": body}

wait_panel(opener, csrf_endpoint)
csrf_token = login_panel(opener, csrf_endpoint, login_url, panel_user, panel_pass)
panel_public_url = build_panel_public_url(server_ip, panel_port, panel_base)
hy2_cert_pin = cert_pin_sha256_hex(f"{cert_dir}/cert.pem")

vless_payload = {
    "enable": True,
    "remark": f"VLESS Reality {vless_sni}",
    "listen": "",
    "port": vless_port,
    "protocol": "vless",
    "expiryTime": 0,
    "settings": {
        "clients": [{
            "id": client_uuid,
            "flow": "xtls-rprx-vision",
            "email": vless_client_email,
            "limitIp": 0,
            "totalGB": 0,
            "expiryTime": 0,
            "enable": True,
            "tgId": 0,
            "subId": "",
        }],
        "decryption": "none",
        "fallbacks": [],
    },
    "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
            "show": False,
            "dest": f"{vless_sni}:443",
            "xver": 0,
            "serverNames": [vless_sni],
            "privateKey": priv,
            "publicKey": pub,
            "shortIds": [short_id],
            "fingerprint": "firefox",
            "spiderX": "/",
        },
        "tcpSettings": {
            "acceptProxyProtocol": False,
            "header": {"type": "none"},
        },
    },
    "sniffing": {
        "enabled": True,
        "destOverride": ["http", "tls", "quic", "fakedns"],
    },
}
vless_resp = post("/panel/api/inbounds/add", vless_payload)
if not vless_resp.get("success", True):
    msg = str(vless_resp.get("msg") or "")
    if "already used" in msg.lower():
        lst = get("/panel/api/inbounds/list")
        for ib in (lst.get("obj") or []):
            try:
                if int(ib.get("port", -1)) == vless_port:
                    post(f"/panel/api/inbounds/del/{ib.get('id')}", {})
            except (TypeError, ValueError):
                continue
        vless_resp = post("/panel/api/inbounds/add", vless_payload)
    if not vless_resp.get("success", True) and vless_resp.get("msg"):
        raise RuntimeError("vless inbound: " + str(vless_resp.get("msg")))

from urllib.parse import quote

vless_uri = (
    f"vless://{client_uuid}@{server_ip}:{vless_port}"
    f"?type=tcp&encryption=none&flow=xtls-rprx-vision&security=reality"
    f"&pbk={quote(pub, safe='')}&fp=firefox&sni={quote(vless_sni, safe='')}&sid={quote(short_id, safe='')}&spx=%2F"
    f"#VLESS-{vless_sni}"
)
hy2_uri = (
    f"hysteria2://{quote(hy2_pass, safe='')}@{server_ip}:{hy2_port}/"
    f"?sni={quote(hy2_sni, safe='')}&pinSHA256={hy2_cert_pin}#{quote('HY2-' + hy2_sni, safe='')}"
)

bundle = {
    "profile": "3x-ui",
    "panel": {
        "url": panel_public_url,
        "username": panel_user,
        "password": panel_pass,
    },
    "vless": {
        "uri": vless_uri,
        "port": vless_port,
        "sni": vless_sni,
        "uuid": client_uuid,
        "public_key": pub,
        "short_id": short_id,
        "email": vless_client_email,
    },
    "hysteria2": {
        "uri": hy2_uri,
        "port": hy2_port,
        "sni": hy2_sni,
        "password": hy2_pass,
        "email": hy2_client_email,
    },
}
print("BUNDLE_JSON:" + json.dumps(bundle, ensure_ascii=False))
PY

systemctl restart x-ui >/dev/null 2>&1 || systemctl restart x-ui.service
for _wait in $(seq 1 30); do
  if ss -tln 2>/dev/null | grep -q ":${VLESS_PORT} " || netstat -tln 2>/dev/null | grep -q ":${VLESS_PORT} "; then
    break
  fi
  sleep 1
done

if command -v ufw >/dev/null 2>&1; then
  ufw allow OpenSSH >/dev/null 2>&1 || ufw allow 22/tcp >/dev/null 2>&1 || true
  ufw allow "${VLESS_PORT}/tcp" >/dev/null 2>&1 || true
  ufw allow "${HY2_PORT}/udp" >/dev/null 2>&1 || true
  ufw allow "${PANEL_PORT}/tcp" >/dev/null 2>&1 || true
  ufw --force enable >/dev/null 2>&1 || true
elif command -v firewall-cmd >/dev/null 2>&1; then
  firewall-cmd --permanent --add-port="${VLESS_PORT}/tcp" >/dev/null 2>&1 || true
  firewall-cmd --permanent --add-port="${HY2_PORT}/udp" >/dev/null 2>&1 || true
  firewall-cmd --permanent --add-port="${PANEL_PORT}/tcp" >/dev/null 2>&1 || true
  firewall-cmd --reload >/dev/null 2>&1 || true
fi
install_hysteria_server() {
  if command -v hysteria >/dev/null 2>&1 && [[ -f /etc/systemd/system/hysteria-server.service ]]; then
    return 0
  fi
  local arch="" os="linux" ver="${HYSTERIA_VERSION:-v2.12.1}" tmp=""
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) fail "unsupported CPU arch for hysteria: $(uname -m)" ;;
  esac
  tmp="$(mktemp)"
  if ! curl -fsSL "https://github.com/apernet/hysteria/releases/download/app/${ver}/hysteria-${os}-${arch}" -o "${tmp}"; then
    curl -fsSL "https://download.hysteria.network/app/${ver}/hysteria-${os}-${arch}" -o "${tmp}" || fail "hysteria binary download failed"
  fi
  install -Dm755 "${tmp}" /usr/local/bin/hysteria
  rm -f "${tmp}"
  if ! id hysteria >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/hysteria --create-home --shell /usr/sbin/nologin hysteria 2>/dev/null \
      || useradd -r -d /var/lib/hysteria -s /sbin/nologin hysteria 2>/dev/null \
      || true
  fi
  mkdir -p /etc/hysteria /var/lib/hysteria
  if [[ ! -f /etc/systemd/system/hysteria-server.service ]]; then
    cat > /etc/systemd/system/hysteria-server.service <<'HYEOF'
[Unit]
Description=Hysteria Server Service (config.yaml)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hysteria server --config /etc/hysteria/config.yaml
WorkingDirectory=/var/lib/hysteria
User=hysteria
Group=hysteria
Environment=HYSTERIA_LOG_LEVEL=info
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
HYEOF
    systemctl daemon-reload
  fi
}
install_hysteria_server

cat > "${CERT_DIR}/config.yaml" <<EOF
listen: :${HY2_PORT}
tls:
  cert: ${CERT_DIR}/cert.pem
  key: ${CERT_DIR}/key.pem
auth:
  type: password
  password: ${HY2_PASS}
masquerade:
  type: proxy
  proxy:
    url: https://www.wikipedia.org/
    rewriteHost: true
EOF

chmod 755 "${CERT_DIR}"
chmod 644 "${CERT_DIR}/cert.pem"
chmod 600 "${CERT_DIR}/key.pem"
if id hysteria >/dev/null 2>&1; then
  chown hysteria:hysteria "${CERT_DIR}/cert.pem" "${CERT_DIR}/key.pem"
fi

if [[ -f /etc/systemd/system/hysteria-server.service ]]; then
  if grep -q '/etc/hysteria/config.yaml' /etc/systemd/system/hysteria-server.service 2>/dev/null; then
    :
  elif grep -q 'config.yaml' /etc/systemd/system/hysteria-server.service 2>/dev/null; then
    sed -i "s#-c .*#server -c ${CERT_DIR}/config.yaml#" /etc/systemd/system/hysteria-server.service 2>/dev/null || true
  fi
  hy_cfg="${CERT_DIR}/config.yaml"
  if [[ "$hy_cfg" != "/etc/hysteria/config.yaml" ]]; then
    ln -sf "$hy_cfg" /etc/hysteria/config.yaml 2>/dev/null || cp -f "$hy_cfg" /etc/hysteria/config.yaml
  fi
  systemctl daemon-reload
fi

systemctl enable hysteria-server >/dev/null 2>&1 || true
systemctl restart hysteria-server >/dev/null 2>&1 || systemctl restart hysteria >/dev/null 2>&1 || fail "hysteria-server failed to start"
for _wait in $(seq 1 30); do
  if ss -uln 2>/dev/null | grep -q ":${HY2_PORT} " || netstat -uln 2>/dev/null | grep -q ":${HY2_PORT} "; then
    break
  fi
  sleep 1
done
if ! ss -uln 2>/dev/null | grep -q ":${HY2_PORT} " && ! netstat -uln 2>/dev/null | grep -q ":${HY2_PORT} "; then
  echo "bootstrap_error: hysteria2 UDP port ${HY2_PORT} not listening" >&2
  exit 1
fi

