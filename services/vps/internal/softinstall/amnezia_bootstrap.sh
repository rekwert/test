#!/usr/bin/env bash
# Official Amnezia AWG (Docker) bootstrap. Mobile-friendly UDP/443. Prints BUNDLE_JSON: line.
set -euo pipefail

SERVER_IP="${SERVER_IP:-}"
AWG_PORT="${AWG_PORT:-443}"
CONTAINER_NAME="${CONTAINER_NAME:-amnezia-awg}"
DOCKERFILE_FOLDER="${DOCKERFILE_FOLDER:-/opt/amnezia/amnezia-awg}"
AMNEZIA_SCRIPTS_BASE="${AMNEZIA_SCRIPTS_BASE:-https://raw.githubusercontent.com/amnezia-vpn/amnezia-client/dev/client/server_scripts}"
CLIENT_NAME="${CLIENT_NAME:-default}"

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
  apt-get install -y curl wget ca-certificates python3 openssl
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y curl wget ca-certificates python3 openssl
elif command -v yum >/dev/null 2>&1; then
  yum install -y curl wget ca-certificates python3 openssl
else
  fail "unsupported package manager (need Ubuntu 24.04 or Debian 12)"
fi

fetch_script() {
  local rel="$1"
  local dest="$2"
  curl -fsSL "${AMNEZIA_SCRIPTS_BASE}/${rel}" -o "$dest"
  chmod +x "$dest" 2>/dev/null || true
}

# Docker (official Amnezia installer script).
if ! command -v docker >/dev/null 2>&1; then
  fetch_script "install_docker.sh" /tmp/amnezia_install_docker.sh
  bash /tmp/amnezia_install_docker.sh
fi
command -v docker >/dev/null 2>&1 || fail "docker not available after install"
docker info >/dev/null 2>&1 || fail "docker daemon not running"

fetch_script "prepare_host.sh" /tmp/amnezia_prepare_host.sh
fetch_script "setup_host_firewall.sh" /tmp/amnezia_setup_host_firewall.sh
fetch_script "install_conntrack.sh" /tmp/amnezia_install_conntrack.sh
export DOCKERFILE_FOLDER
bash /tmp/amnezia_prepare_host.sh
bash /tmp/amnezia_setup_host_firewall.sh || true
bash /tmp/amnezia_install_conntrack.sh || true

mkdir -p "$DOCKERFILE_FOLDER"
fetch_script "awg/Dockerfile" "${DOCKERFILE_FOLDER}/Dockerfile"
fetch_script "awg/start.sh" "${DOCKERFILE_FOLDER}/start.sh"
fetch_script "awg/configure_container.sh" "${DOCKERFILE_FOLDER}/configure_container.sh"
fetch_script "awg/template.conf" "${DOCKERFILE_FOLDER}/template.conf"
chmod +x "${DOCKERFILE_FOLDER}/start.sh"

cat > "${DOCKERFILE_FOLDER}/Dockerfile" <<'DOCKEREOF'
FROM amneziavpn/amneziawg-go:latest
LABEL maintainer="AmneziaVPN"
RUN apk add --no-cache bash curl dumb-init && apk --update upgrade --no-cache
RUN mkdir -p /opt/amnezia
COPY start.sh /opt/amnezia/start.sh
RUN chmod a+x /opt/amnezia/start.sh
ENTRYPOINT [ "dumb-init", "/opt/amnezia/start.sh" ]
CMD [ "" ]
DOCKEREOF

apply_awg_iptables() {
  docker exec "$CONTAINER_NAME" env \
    AWG_SUBNET_IP="$AWG_SUBNET_IP" \
    WIREGUARD_SUBNET_CIDR="$WIREGUARD_SUBNET_CIDR" \
    sh -c '
      iptables -C INPUT -i awg0 -j ACCEPT 2>/dev/null || iptables -A INPUT -i awg0 -j ACCEPT
      iptables -C FORWARD -i awg0 -j ACCEPT 2>/dev/null || iptables -A FORWARD -i awg0 -j ACCEPT
      iptables -C OUTPUT -o awg0 -j ACCEPT 2>/dev/null || iptables -A OUTPUT -o awg0 -j ACCEPT
      iptables -C FORWARD -i awg0 -o eth0 -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -j ACCEPT 2>/dev/null || \
        iptables -A FORWARD -i awg0 -o eth0 -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -j ACCEPT
      iptables -C FORWARD -i awg0 -o eth1 -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -j ACCEPT 2>/dev/null || \
        iptables -A FORWARD -i awg0 -o eth1 -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -j ACCEPT
      iptables -C FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || \
        iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT
      iptables -t nat -C POSTROUTING -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -o eth0 -j MASQUERADE 2>/dev/null || \
        iptables -t nat -A POSTROUTING -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -o eth0 -j MASQUERADE
      iptables -t nat -C POSTROUTING -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -o eth1 -j MASQUERADE 2>/dev/null || \
        iptables -t nat -A POSTROUTING -s "$AWG_SUBNET_IP/$WIREGUARD_SUBNET_CIDR" -o eth1 -j MASQUERADE
    '
}

if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
fi

# Generate AWG 2.0 obfuscation params before container start (start.sh needs subnet env).
export AWG_PORT
python3 - <<'PY' > /tmp/amnezia_awg.env
import os, random
def rand_header():
    return str(random.randint(1, 2147483647))
pairs = {
    "AWG_SUBNET_IP": "10.8.1.1",
    "WIREGUARD_SUBNET_CIDR": "24",
    "AWG_SERVER_PORT": os.environ.get("AWG_PORT", "443"),
    "JUNK_PACKET_COUNT": str(random.randint(4, 7)),
    "JUNK_PACKET_MIN_SIZE": "10",
    "JUNK_PACKET_MAX_SIZE": "50",
    "INIT_PACKET_JUNK_SIZE": str(random.randint(15, 150)),
    "RESPONSE_PACKET_JUNK_SIZE": str(random.randint(15, 150)),
    "COOKIE_REPLY_PACKET_JUNK_SIZE": str(random.randint(1, 64)),
    "TRANSPORT_PACKET_JUNK_SIZE": str(random.randint(1, 20)),
    "INIT_PACKET_MAGIC_HEADER": rand_header(),
    "RESPONSE_PACKET_MAGIC_HEADER": rand_header(),
    "UNDERLOAD_PACKET_MAGIC_HEADER": rand_header(),
    "TRANSPORT_PACKET_MAGIC_HEADER": rand_header(),
    "CONTENT_PADDING_ADDITION": str(random.randint(0, 8)),
    "REKEY_AFTER_TIME": "0",
    "REKEY_TIMEOUT": "0",
    "REJECT_AFTER_TIME": "0",
    "KEEPALIVE_TIMEOUT": "0",
    "MAX_HANDSHAKE_ATTEMPTS": "0",
    "RANDOM_TRAILERS": "0",
    "DISABLE_COOKIES": "0",
    "PERSISTENT_KEEPALIVE": "25",
    "PRIMARY_DNS": "1.1.1.1",
    "SECONDARY_DNS": "1.0.0.1",
    "CLIENT_IP": "10.8.1.2",
}
for k, v in pairs.items():
    print(f"{k}={v}")
PY

set -a
# shellcheck disable=SC1091
source /tmp/amnezia_awg.env
set +a
export CONTAINER_NAME DOCKERFILE_FOLDER AWG_SERVER_PORT="$AWG_PORT" SERVER_IP_ADDRESS="$SERVER_IP"

docker build --pull -t "$CONTAINER_NAME" "$DOCKERFILE_FOLDER"

docker run -d \
  --log-driver none \
  --restart always \
  --privileged \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  -p "${AWG_PORT}:${AWG_PORT}/udp" \
  -v /lib/modules:/lib/modules \
  --sysctl="net.ipv4.conf.all.src_valid_mark=1" \
  -e AWG_SUBNET_IP="$AWG_SUBNET_IP" \
  -e WIREGUARD_SUBNET_CIDR="$WIREGUARD_SUBNET_CIDR" \
  -e AWG_SERVER_PORT="$AWG_SERVER_PORT" \
  -e SERVER_IP_ADDRESS="$SERVER_IP_ADDRESS" \
  --name "$CONTAINER_NAME" \
  "$CONTAINER_NAME"

docker network connect amnezia-dns-net "$CONTAINER_NAME" 2>/dev/null || true

# AWG expects a WireGuard-style base64 key (awg genkey), not a hex string.
HEADER_PROTECTION_KEY="$(docker exec "$CONTAINER_NAME" awg genkey | tr -d '\r\n')"
[[ -n "$HEADER_PROTECTION_KEY" ]] || fail "awg genkey failed for header protection"
echo "HEADER_PROTECTION_KEY=${HEADER_PROTECTION_KEY}" >> /tmp/amnezia_awg.env

set -a
# shellcheck disable=SC1091
source /tmp/amnezia_awg.env
set +a
export AWG_SERVER_PORT="$AWG_PORT"
export SERVER_IP_ADDRESS="$SERVER_IP"

docker cp "${DOCKERFILE_FOLDER}/configure_container.sh" "${CONTAINER_NAME}:/opt/amnezia/configure_container.sh"
docker exec "$CONTAINER_NAME" env \
  AWG_SUBNET_IP="$AWG_SUBNET_IP" \
  WIREGUARD_SUBNET_CIDR="$WIREGUARD_SUBNET_CIDR" \
  AWG_SERVER_PORT="$AWG_SERVER_PORT" \
  JUNK_PACKET_COUNT="$JUNK_PACKET_COUNT" \
  JUNK_PACKET_MIN_SIZE="$JUNK_PACKET_MIN_SIZE" \
  JUNK_PACKET_MAX_SIZE="$JUNK_PACKET_MAX_SIZE" \
  INIT_PACKET_JUNK_SIZE="$INIT_PACKET_JUNK_SIZE" \
  RESPONSE_PACKET_JUNK_SIZE="$RESPONSE_PACKET_JUNK_SIZE" \
  COOKIE_REPLY_PACKET_JUNK_SIZE="$COOKIE_REPLY_PACKET_JUNK_SIZE" \
  TRANSPORT_PACKET_JUNK_SIZE="$TRANSPORT_PACKET_JUNK_SIZE" \
  INIT_PACKET_MAGIC_HEADER="$INIT_PACKET_MAGIC_HEADER" \
  RESPONSE_PACKET_MAGIC_HEADER="$RESPONSE_PACKET_MAGIC_HEADER" \
  UNDERLOAD_PACKET_MAGIC_HEADER="$UNDERLOAD_PACKET_MAGIC_HEADER" \
  TRANSPORT_PACKET_MAGIC_HEADER="$TRANSPORT_PACKET_MAGIC_HEADER" \
  HEADER_PROTECTION_KEY="$HEADER_PROTECTION_KEY" \
  CONTENT_PADDING_ADDITION="$CONTENT_PADDING_ADDITION" \
  REKEY_AFTER_TIME="$REKEY_AFTER_TIME" \
  REKEY_TIMEOUT="$REKEY_TIMEOUT" \
  REJECT_AFTER_TIME="$REJECT_AFTER_TIME" \
  KEEPALIVE_TIMEOUT="$KEEPALIVE_TIMEOUT" \
  MAX_HANDSHAKE_ATTEMPTS="$MAX_HANDSHAKE_ATTEMPTS" \
  RANDOM_TRAILERS="$RANDOM_TRAILERS" \
  DISABLE_COOKIES="$DISABLE_COOKIES" \
  bash -c 'chmod +x /opt/amnezia/configure_container.sh && /opt/amnezia/configure_container.sh'

SERVER_PUB="$(docker exec "$CONTAINER_NAME" cat /opt/amnezia/awg/wireguard_server_public_key.key | tr -d '\r\n')"
PSK="$(docker exec "$CONTAINER_NAME" cat /opt/amnezia/awg/wireguard_psk.key | tr -d '\r\n')"
CLIENT_PRIV="$(docker exec "$CONTAINER_NAME" awg genkey | tr -d '\r\n')"
CLIENT_PUB="$(echo "$CLIENT_PRIV" | docker exec -i "$CONTAINER_NAME" awg pubkey | tr -d '\r\n')"

docker exec "$CONTAINER_NAME" bash -c "cat >> /opt/amnezia/awg/awg0.conf <<EOF

[Peer]
PublicKey = ${CLIENT_PUB}
PresharedKey = ${PSK}
AllowedIPs = ${CLIENT_IP}/32
EOF
awg-quick down /opt/amnezia/awg/awg0.conf 2>/dev/null || true
awg-quick up /opt/amnezia/awg/awg0.conf
"

apply_awg_iptables

export SERVER_PUB PSK CLIENT_PRIV CLIENT_PUB SERVER_IP AWG_PORT CLIENT_NAME
export AWG_ENV_JSON="$(python3 - <<'PY'
import json, os
keys = [
    "AWG_SUBNET_IP", "WIREGUARD_SUBNET_CIDR", "AWG_SERVER_PORT",
    "JUNK_PACKET_COUNT", "JUNK_PACKET_MIN_SIZE", "JUNK_PACKET_MAX_SIZE",
    "INIT_PACKET_JUNK_SIZE", "RESPONSE_PACKET_JUNK_SIZE",
    "COOKIE_REPLY_PACKET_JUNK_SIZE", "TRANSPORT_PACKET_JUNK_SIZE",
    "INIT_PACKET_MAGIC_HEADER", "RESPONSE_PACKET_MAGIC_HEADER",
    "UNDERLOAD_PACKET_MAGIC_HEADER", "TRANSPORT_PACKET_MAGIC_HEADER",
    "HEADER_PROTECTION_KEY", "CONTENT_PADDING_ADDITION",
    "REKEY_AFTER_TIME", "REKEY_TIMEOUT", "REJECT_AFTER_TIME",
    "KEEPALIVE_TIMEOUT", "MAX_HANDSHAKE_ATTEMPTS", "RANDOM_TRAILERS",
    "DISABLE_COOKIES", "PERSISTENT_KEEPALIVE", "PRIMARY_DNS", "SECONDARY_DNS",
    "CLIENT_IP",
]
cfg = {k: os.environ.get(k, "") for k in keys}
print(json.dumps(cfg))
PY
)"

python3 - <<'PY'
import json, os, struct, zlib, base64, textwrap

def qcompress(data: bytes) -> bytes:
    return struct.pack(">I", len(data)) + zlib.compress(data, 8)

def vpn_uri(obj: dict) -> str:
    raw = json.dumps(obj, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    payload = qcompress(raw)
    b64 = base64.urlsafe_b64encode(payload).decode().rstrip("=")
    return "vpn://" + b64

cfg = json.loads(os.environ["AWG_ENV_JSON"])
server_ip = os.environ["SERVER_IP"]
port = int(os.environ.get("AWG_PORT", "443"))
client_name = os.environ.get("CLIENT_NAME", "default")
server_pub = os.environ["SERVER_PUB"]
psk = os.environ["PSK"]
client_priv = os.environ["CLIENT_PRIV"]
client_pub = os.environ["CLIENT_PUB"]
client_ip = cfg["CLIENT_IP"]

client_conf = f"""[Interface]
Address = {client_ip}/32
DNS = {cfg['PRIMARY_DNS']}, {cfg['SECONDARY_DNS']}
PrivateKey = {client_priv}
MTU = 1280
Jc = {cfg['JUNK_PACKET_COUNT']}
Jmin = {cfg['JUNK_PACKET_MIN_SIZE']}
Jmax = {cfg['JUNK_PACKET_MAX_SIZE']}
S1 = {cfg['INIT_PACKET_JUNK_SIZE']}
S2 = {cfg['RESPONSE_PACKET_JUNK_SIZE']}
S3 = {cfg['COOKIE_REPLY_PACKET_JUNK_SIZE']}
S4 = {cfg['TRANSPORT_PACKET_JUNK_SIZE']}
H1 = {cfg['INIT_PACKET_MAGIC_HEADER']}
H2 = {cfg['RESPONSE_PACKET_MAGIC_HEADER']}
H3 = {cfg['UNDERLOAD_PACKET_MAGIC_HEADER']}
H4 = {cfg['TRANSPORT_PACKET_MAGIC_HEADER']}
HeaderProtectionKey = {cfg['HEADER_PROTECTION_KEY']}
ContentPaddingAddition = {cfg['CONTENT_PADDING_ADDITION']}
RekeyAfterTime = {cfg['REKEY_AFTER_TIME']}
RekeyTimeout = {cfg['REKEY_TIMEOUT']}
RejectAfterTime = {cfg['REJECT_AFTER_TIME']}
KeepaliveTimeout = {cfg['KEEPALIVE_TIMEOUT']}
MaxHandshakeAttempts = {cfg['MAX_HANDSHAKE_ATTEMPTS']}
RandomTrailers = {cfg['RANDOM_TRAILERS']}
DisableCookies = {cfg['DISABLE_COOKIES']}

[Peer]
PublicKey = {server_pub}
PresharedKey = {psk}
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = {server_ip}:{port}
PersistentKeepalive = {cfg['PERSISTENT_KEEPALIVE']}
"""

last_config = {
    "allowed_ips": ["0.0.0.0/0", "::/0"],
    "client_ip": f"{client_ip}/32",
    "client_priv_key": client_priv,
    "client_pub_key": client_pub,
    "config": client_conf,
    "hostName": server_ip,
    "mtu": "1280",
    "persistent_keep_alive": cfg["PERSISTENT_KEEPALIVE"],
    "port": port,
    "psk_key": psk,
    "server_pub_key": server_pub,
    "Jc": cfg["JUNK_PACKET_COUNT"],
    "Jmin": cfg["JUNK_PACKET_MIN_SIZE"],
    "Jmax": cfg["JUNK_PACKET_MAX_SIZE"],
    "S1": cfg["INIT_PACKET_JUNK_SIZE"],
    "S2": cfg["RESPONSE_PACKET_JUNK_SIZE"],
    "S3": cfg["COOKIE_REPLY_PACKET_JUNK_SIZE"],
    "S4": cfg["TRANSPORT_PACKET_JUNK_SIZE"],
    "H1": cfg["INIT_PACKET_MAGIC_HEADER"],
    "H2": cfg["RESPONSE_PACKET_MAGIC_HEADER"],
    "H3": cfg["UNDERLOAD_PACKET_MAGIC_HEADER"],
    "H4": cfg["TRANSPORT_PACKET_MAGIC_HEADER"],
    "HeaderProtectionKey": cfg["HEADER_PROTECTION_KEY"],
    "ContentPaddingAddition": cfg["CONTENT_PADDING_ADDITION"],
}

server_config = {
    "containers": [{
        "container": "amnezia-awg",
        "awg": {
            "isThirdPartyConfig": True,
            "last_config": json.dumps(last_config, ensure_ascii=False, separators=(",", ":")),
            "port": str(port),
            "protocol_version": "2",
            "transport_proto": "udp",
        },
    }],
    "defaultContainer": "amnezia-awg",
    "description": client_name,
    "dns1": cfg["PRIMARY_DNS"],
    "dns2": cfg["SECONDARY_DNS"],
    "hostName": server_ip,
}

uri = vpn_uri(server_config)
bundle = {
    "profile": "amnezia",
    "amnezia": {
        "vpn_uri": uri,
        "client_name": client_name,
        "port": port,
    },
}
print("BUNDLE_JSON:" + json.dumps(bundle, ensure_ascii=False, separators=(",", ":")))
PY
