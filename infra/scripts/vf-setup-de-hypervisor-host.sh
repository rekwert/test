#!/usr/bin/env bash
# Prepare DE Hostkey hypervisor host: hostname, br0, VirtFusion hypervisor agent.
# Run from back server (needs sshpass). Usage:
#   export DE_SSH_PASS='...'
#   bash vf-setup-de-hypervisor-host.sh
set -euo pipefail

DE_IP="${DE_IP:-185.84.224.84}"
DE_GW="${DE_GW:-185.84.224.65}"
DE_ADDR="${DE_ADDR:-185.84.224.84}"
DE_MASK="${DE_MASK:-255.255.255.192}"
DE_HOSTNAME="${DE_HOSTNAME:-DE-prosto}"
: "${DE_SSH_PASS:?set DE_SSH_PASS}"

ssh_de() {
  SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 "root@${DE_IP}" "$@"
}

echo "=== 1. Connectivity ==="
ssh_de 'echo OK; uname -a; head -3 /etc/os-release; ip -br a'

echo "=== 2. Hostname ==="
ssh_de "hostnamectl set-hostname ${DE_HOSTNAME}; hostname"

echo "=== 3. br0 bridge (Hostkey-style) — requires live console if SSH drops ==="
if [[ "${DE_SKIP_BR0:-0}" == "1" ]]; then
  echo "DE_SKIP_BR0=1 — keeping current eno1 routing; set DE_SKIP_BR0=0 when on KVM console"
else
ssh_de 'bash -s' <<REMOTE
set -euo pipefail
PRIMARY_IP=${DE_ADDR}
GW=${DE_GW}
MASK=${DE_MASK}
NIC=\$(ip -o -4 addr show | awk -v ip="\$PRIMARY_IP" '\$0 ~ ip {print \$2; exit}')
if [[ -z "\${NIC:-}" || "\$NIC" == "br0" ]]; then
  NIC=\$(ip -o link show | awk -F': ' '\$2 !~ /^(lo|br|docker|veth|virbr)/ {print \$2; exit}')
fi
echo "NIC=\$NIC"
if ip addr show br0 2>/dev/null | grep -q "\$PRIMARY_IP"; then
  echo "br0 already has public IP"
else
  mkdir -p /home/vf-data/disk
  chmod 755 /home/vf-data /home/vf-data/disk
  apt-get update -y
  apt-get install -y bridge-utils curl ifupdown2 sshpass 2>/dev/null || apt-get install -y bridge-utils curl
  cp -a /etc/network/interfaces /etc/network/interfaces.bak.de-prosto.\$(date +%s) 2>/dev/null || true
  cat >/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback

auto \${NIC}
iface \${NIC} inet manual

auto br0
iface br0 inet static
  address \${PRIMARY_IP}
  netmask \${MASK}
  gateway \${GW}
  bridge_ports \${NIC}
  bridge_stp off
  bridge_fd 0
  dns-nameservers 1.1.1.1 8.8.8.8
EOF
  echo "Applying br0 — SSH may drop for ~30s..."
  ifdown "\${NIC}" 2>/dev/null || true
  ifdown br0 2>/dev/null || true
  ifup br0 || true
  sleep 3
fi
ip addr show br0 | head -15 || ip addr show eno1 | head -15
ping -c2 -W3 \${GW} || true
ping -c2 -W3 8.8.8.8 || true
REMOTE
fi

echo "=== 4. VirtFusion hypervisor agent ==="
ssh_de 'bash -s' <<'REMOTE'
set -euo pipefail
if [ -d /opt/virtfusion/app/hypervisor ]; then
  echo "VirtFusion hypervisor already installed"
  supervisorctl status 2>/dev/null | head -10 || systemctl is-active libvirtd 2>/dev/null || true
  exit 0
fi
curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
sleep 5
supervisorctl status 2>/dev/null | head -10 || true
systemctl is-active libvirtd 2>/dev/null || true
ls -la /opt/virtfusion/app/hypervisor 2>/dev/null | head -5
REMOTE

echo "DE_HOST_SETUP_DONE"
