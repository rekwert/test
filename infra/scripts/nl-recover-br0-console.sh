#!/bin/bash
# Paste into Hostkey HTML5 console if NL is offline after br0 migrate.
# Restores public IP on br0 (or falls back to enp2s0f0).
set -euo pipefail
PRIMARY_IP=66.248.206.14
GW=66.248.206.1
NIC=enp2s0f0
MGMT_IP=172.18.5.184

apt-get install -y bridge-utils >/dev/null 2>&1 || true

ip link add name br0 type bridge 2>/dev/null || true
ip link set "$NIC" up
ip link set "$NIC" master br0 2>/dev/null || true
ip link set br0 up
ip addr flush dev "$NIC" 2>/dev/null || true
ip addr add "${PRIMARY_IP}/24" dev br0 2>/dev/null || true
ip addr add "${MGMT_IP}/24" dev br0 2>/dev/null || true
ip route replace default via "$GW" dev br0 2>/dev/null || \
  ip route add default via "$GW" dev br0 2>/dev/null || true

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
  post-up ip addr add ${MGMT_IP}/24 dev br0 || true
  dns-nameservers 1.1.1.1 8.8.8.8
EOF

ip -br a
ip route
ping -c 2 -W 2 8.8.8.8 || true
echo RECOVERED
