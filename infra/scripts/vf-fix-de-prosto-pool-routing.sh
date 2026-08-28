#!/usr/bin/env bash
# Fix DE-prosto (212.102.227.6) routed pool for existing VMs — no reboot, no libvirt changes.
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

DE_HV_IP=212.102.227.6
DE_POOL_CIDR=212.102.227.0/24
DE_POOL_GATEWAY=212.102.227.1

echo "=== Before ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_HV_IP" \
  'ip -br addr show br0; ip route | grep 212.102'

echo "=== Apply pool gateway on br0 ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_HV_IP" "bash -s" <<REMOTE
set -euo pipefail
CIDR='$DE_POOL_CIDR'
GW='$DE_POOL_GATEWAY'
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
sysctl -w net.ipv4.ip_forward=1 net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1 >/dev/null
ip route replace \${CIDR} dev br0 scope link 2>/dev/null || ip route add \${CIDR} dev br0 scope link
ip addr show dev br0 | grep -q "\${GW}/" || ip addr add \${GW}/32 dev br0
command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
IF=/etc/network/interfaces
if [ -f "\$IF" ] && ! grep -q "vf-routed-\${GW}" "\$IF"; then
  cat >>"\$IF" <<IFACE

# vf-routed-\${GW}
up ip route replace \${CIDR} dev br0 scope link || true
up ip addr add \${GW}/32 dev br0 || true
IFACE
fi
echo "br0:"; ip -br addr show br0
echo "route:"; ip route | grep 212.102
echo "sysctl:"; sysctl net.ipv4.ip_forward net.ipv4.conf.br0.proxy_arp
echo "VMs running:"; virsh list --state-running | grep -c running || true
REMOTE

echo ""
echo "=== Ping VMs from HV ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_HV_IP" \
  'for ip in 212.102.227.3 212.102.227.4 212.102.227.10 212.102.227.15 212.102.227.30 212.102.227.39; do ping -c1 -W1 $ip >/dev/null 2>&1 && echo $ip OK || echo $ip FAIL; done'

echo ""
echo "=== Ping VMs from back (external) ==="
source /opt/testVPStrade/infra/docker/.env
for ip in 212.102.227.3 212.102.227.4 212.102.227.10 212.102.227.15 212.102.227.30 212.102.227.39; do
  ping -c1 -W2 "$ip" >/dev/null 2>&1 && echo "$ip EXT_OK" || echo "$ip EXT_FAIL"
done

echo "DE_PROSTO_POOL_ROUTING_DONE"
