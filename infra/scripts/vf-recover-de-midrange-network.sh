#!/usr/bin/env bash
# Recover DE-midrange (66.151.40.165) network — run ON THE SERVER via Hostkey KVM console if SSH is down.
set -euo pipefail
PRIMARY_IP=66.151.40.165
GW=66.151.40.1
MASK=255.255.255.0
POOL_GW=212.102.227.1
POOL_CIDR=212.102.227.0/24

NIC=$(ip -o link show | awk -F': ' '$2 !~ /^(lo|br|docker|veth|virbr)/ {print $2; exit}')
hostnamectl set-hostname DE-midrange
mkdir -p /home/vf-data/disk && chmod 755 /home/vf-data /home/vf-data/disk
apt-get update -y && apt-get install -y bridge-utils ifupdown2 curl sshpass

cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback
auto ${NIC}
iface ${NIC} inet manual
auto br0
iface br0 inet static
  address ${PRIMARY_IP}
  netmask ${MASK}
  gateway ${GW}
  bridge_ports ${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
up ip route replace ${POOL_CIDR} dev br0 scope link
up ip addr add ${POOL_GW}/32 dev br0 || true
EOF

ifdown "$NIC" 2>/dev/null || true
ifup br0
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1
[ -d /opt/virtfusion/app/hypervisor ] || curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
echo "DE_MIDRANGE_NETWORK_OK — ask ops to re-commission HV on NL panel"
