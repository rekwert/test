#!/usr/bin/env bash
# EMERGENCY: run on DE-midrange 66.151.40.165 via Hostkey KVM if SSH is down.
# Restores working network (do NOT use 66.151.40.129 — breaks this host).
set -euo pipefail
NIC=$(ip -o link show | awk -F': ' '$2 !~ /^(lo|br|docker|veth|virbr)/ {print $2; exit}')
cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback
auto ${NIC}
iface ${NIC} inet manual
auto br0
iface br0 inet static
  address 66.151.40.165
  netmask 255.255.255.0
  gateway 66.151.40.1
  bridge_ports ${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
up ip route replace 212.102.227.0/24 dev br0 scope link
up ip addr add 212.102.227.1/32 dev br0 || true
EOF
ip addr del 172.21.5.189/24 dev br0 2>/dev/null || true
ifdown br0 2>/dev/null || true
ifup br0
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT
echo "SSH should work now: $(ip route | head -3)"
