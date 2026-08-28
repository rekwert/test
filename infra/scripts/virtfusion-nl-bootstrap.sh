#!/usr/bin/env bash
# VirtFusion NL bare-metal bootstrap (Hostkey 66.248.206.14)
# Run as root on fresh Debian 12 after Hostkey reinstall.
set -euo pipefail

LOG=/root/virtfusion-bootstrap.log
exec > >(tee -a "$LOG") 2>&1

PRIMARY_IP="${PRIMARY_IP:-66.248.206.14}"
GATEWAY="${GATEWAY:-66.248.206.1}"
NETMASK="${NETMASK:-255.255.255.0}"
IFACE="${IFACE:-enp2s0f0}"
BRIDGE="${BRIDGE:-br0}"
CONTROL_IP="${CONTROL_IP:-66.248.206.21}"
POOL_IPS="${POOL_IPS:-66.248.206.40,66.248.206.61}"

echo "=== VirtFusion NL bootstrap started $(date -Is) ==="

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y curl bridge-utils ifupdown2 lxc debootstrap

# Bridged network for VirtFusion (Hostkey bare metal)
if ! grep -q "iface ${BRIDGE} inet static" /etc/network/interfaces 2>/dev/null; then
  cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback

auto ${IFACE}
iface ${IFACE} inet manual

auto ${BRIDGE}
iface ${BRIDGE} inet static
    address ${PRIMARY_IP}
    netmask ${NETMASK}
    gateway ${GATEWAY}
    dns-nameservers 1.1.1.1 8.8.8.8
    bridge_ports ${IFACE}
    bridge_stp off
    bridge_fd 0
EOF
  systemctl restart networking || true
fi

echo "=== Installing VirtFusion hypervisor ==="
( set -euo pipefail
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
)

echo "=== Installing VirtFusion control (trial: same host) ==="
curl -fsSL https://install.virtfusion.net/install-control-debian-12.sh | sh -s -- --verbose

echo "=== Bootstrap finished $(date -Is) ==="
echo "Next manual steps in VF panel:"
echo "  1. Activate evaluation license"
echo "  2. Add hypervisor ${PRIMARY_IP}"
echo "  3. Create packages VPS-1..VPS-4"
echo "  4. Add IP pool: ${POOL_IPS}"
echo "  5. Generate API token -> update Back .env"
echo "Credentials (if printed by installer) are in ${LOG}"
