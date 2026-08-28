#!/usr/bin/env bash
# Persist Hetzner routed /24 gateway on br0 for GB and DE hypervisors.
# Without GW on br0 + ufw allow routed, guest VMs cannot reach SSH and provision hangs.
#
# Run on back server (198.13.189.75):
#   bash /opt/testVPStrade/infra/scripts/vf-fix-gb-de-gateway-br0.sh
#
# Env overrides (optional):
#   GB_HV_IP, GB_SSH_PASS, GB_POOL_CIDR, GB_POOL_GATEWAY
#   DE_HV_IP, DE_SSH_PASS, DE_POOL_CIDR, DE_POOL_GATEWAY
set -euo pipefail

GB_HV_IP="${GB_HV_IP:-212.108.83.47}"
GB_SSH_PASS="${GB_SSH_PASS:-MYT03f-1Ab}"
GB_POOL_CIDR="${GB_POOL_CIDR:-91.108.247.0/24}"
GB_POOL_GATEWAY="${GB_POOL_GATEWAY:-91.108.247.1}"

DE_HV_IP="${DE_HV_IP:-185.84.224.84}"
DE_SSH_PASS="${DE_SSH_PASS:-tV6m%91rRi}"
DE_POOL_CIDR="${DE_POOL_CIDR:-212.102.227.0/24}"
DE_POOL_GATEWAY="${DE_POOL_GATEWAY:-212.102.227.1}"

setup_gw() {
  local ip=$1 pass=$2 cidr=$3 gw=$4 label=$5
  export SSHPASS="$pass"
  echo "=== $label ($ip): gateway $gw on br0 ==="
  sshpass -e ssh -o StrictHostKeyChecking=no "root@${ip}" "bash -s" <<REMOTE
set -euo pipefail
CIDR='$cidr'
GW='$gw'
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.proxy_arp=1
sysctl -w net.ipv4.conf.br0.proxy_arp=1
ip route replace \${CIDR} dev br0 scope link 2>/dev/null || ip route add \${CIDR} dev br0 scope link
ip addr show dev br0 | grep -q "\${GW}/" || ip addr add \${GW}/32 dev br0
command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
IF=/etc/network/interfaces
if [ -f "\$IF" ] && ! grep -q "vf-routed-\${GW}" "\$IF"; then
  cat >>"\$IF" <<IFACE

# vf-routed-\${GW}
auto br0:vf-\${GW//./-}
iface br0:vf-\${GW//./-} inet static
    address \${GW}
    netmask 255.255.255.255
    post-up ip route replace \${CIDR} dev br0 scope link || true
    pre-down ip addr del \${GW}/32 dev br0 || true
IFACE
fi
echo "br0:"; ip -br addr show br0
echo "route:"; ip route | grep "\${CIDR%%/*}" || true
echo "ufw:"; ufw status 2>/dev/null | grep -E 'Status|routed' || true
echo "sysctl:"; sysctl net.ipv4.ip_forward net.ipv4.conf.br0.proxy_arp
REMOTE
}

setup_gw "$GB_HV_IP" "$GB_SSH_PASS" "$GB_POOL_CIDR" "$GB_POOL_GATEWAY" GB
setup_gw "$DE_HV_IP" "$DE_SSH_PASS" "$DE_POOL_CIDR" "$DE_POOL_GATEWAY" DE

echo ""
echo "=== verify from backend ==="
for ip in 91.108.247.2 212.102.227.2; do
  echo -n "ping $ip: "
  ping -c 1 -W 3 "$ip" >/dev/null 2>&1 && echo OK || echo skip/no-vm
done

echo "VF_GB_DE_GATEWAY_BR0_DONE"
