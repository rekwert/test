#!/bin/bash
# Recovery for FI hypervisor if network migration dropped SSH (Hetzner #3047826).
# Run from Hetzner KVM/Rescue console as root.
set -euo pipefail

echo "=== restore eth0 primary IP ==="
cat >/etc/network/interfaces <<'IFACE'
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 95.216.1.155
    netmask 255.255.255.192
    gateway 95.216.1.129
    dns-nameservers 185.12.64.1 185.12.64.2
    post-up ip route add 135.181.123.152/29 dev br0 || true
    pre-down ip route del 135.181.123.152/29 dev br0 || true

iface eth0 inet6 static
    address 2a01:4f9:2a:1ec::2
    netmask 64
    gateway fe80::1
IFACE

# Remove broken linux bridge if present
ip link set eth0 nomaster 2>/dev/null || true
if ip link show br0 type bridge >/dev/null 2>&1; then
  ip link set br0 down 2>/dev/null || true
  ip link del br0 2>/dev/null || true
fi

ifdown --force eth0 2>/dev/null || true
ifup eth0

echo "=== libvirt routed br0 ==="
virsh net-destroy br0 2>/dev/null || true
virsh net-undefine br0 2>/dev/null || true
cat >/root/network.xml <<'XML'
<network>
  <name>br0</name>
  <forward mode='route' dev='eth0'/>
  <bridge name='br0' stp='on' delay='0'/>
  <ip address='135.181.123.1' netmask='255.255.255.0'/>
</network>
XML
virsh net-define /root/network.xml
virsh net-autostart br0
virsh net-start br0

sysctl -w net.ipv4.ip_forward=1
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-hetzner-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=0
net.ipv4.conf.eth0.proxy_arp=0
net.ipv4.conf.br0.proxy_arp=0
SYS
sysctl -p /etc/sysctl.d/99-vf-hetzner-routed.conf

ip route add 135.181.123.152/29 dev br0 2>/dev/null || true

echo "=== verify ==="
ip -br addr
ip r
virsh net-list --all
echo "eth0 MAC: $(cat /sys/class/net/eth0/address)"
echo RECOVERY_DONE
