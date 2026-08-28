#!/usr/bin/env bash
# Install VirtFusion on empty NL (66.248.206.14) from the back server.
# Usage:
#   export NL_PASS='root-password-from-hostkey'
#   bash /opt/testVPStrade/infra/scripts/vf-install-empty-nl-from-back.sh
set -euo pipefail

NL_HOST="${NL_HOST:-66.248.206.14}"
: "${NL_PASS:?set NL_PASS to NL root password}"

ssh_nl() {
  sshpass -p "$NL_PASS" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 "root@${NL_HOST}" "$@"
}

scp_nl() {
  sshpass -p "$NL_PASS" scp -o StrictHostKeyChecking=no "$@"
}

echo "=== 1. Connectivity ==="
ssh_nl 'echo OK; uname -a; head -3 /etc/os-release; ip -br a'

echo "=== 2. Ensure br0 (Hostkey-style) ==="
ssh_nl 'bash -s' <<'REMOTE'
set -euo pipefail
PRIMARY_IP=66.248.206.14
GW=66.248.206.1
# detect NIC carrying the public IP or first non-lo ethernet
NIC=$(ip -o -4 addr show | awk -v ip="$PRIMARY_IP" '$0 ~ ip {print $2; exit}')
if [[ -z "${NIC:-}" || "$NIC" == "br0" ]]; then
  NIC=$(ip -o link show | awk -F': ' '$2 !~ /^(lo|br|docker|veth|virbr)/ {print $2; exit}')
fi
echo "NIC=$NIC"
if ip addr show br0 2>/dev/null | grep -q "$PRIMARY_IP"; then
  echo "br0 already OK"
  exit 0
fi
mkdir -p /home/vf-data/disk
chmod 755 /home/vf-data /home/vf-data/disk
apt-get update -y
apt-get install -y bridge-utils curl ifupdown2 || apt-get install -y bridge-utils curl
cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback

auto ${NIC}
iface ${NIC} inet manual

auto br0
iface br0 inet static
  address ${PRIMARY_IP}
  netmask 255.255.255.0
  gateway ${GW}
  bridge_ports ${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
EOF
ifdown "${NIC}" 2>/dev/null || true
ifup br0 || true
ip addr show br0 | head -20
REMOTE

echo "=== 3. Upload + run vf-full-reinstall.sh ==="
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
scp_nl "${REPO_ROOT}/infra/scripts/vf-full-reinstall.sh" "root@${NL_HOST}:/root/vf-full-reinstall.sh"
ssh_nl 'chmod +x /root/vf-full-reinstall.sh; bash /root/vf-full-reinstall.sh'

echo "=== 4. Post-install health ==="
ssh_nl 'curl -sk -o /dev/null -w "login:%{http_code}\n" https://127.0.0.1/login || true; systemctl is-active vf-nginx vf-php8-fpm libvirtd 2>/dev/null || true; ls -lt /root/virtfusion-reinstall-*.log 2>/dev/null | head -1'

echo "DONE. Open https://${NL_HOST}/login — password is in the install log on NL."
echo "Then finish panel setup per infra/scripts/VF-REINSTALL-RUNBOOK.md"
