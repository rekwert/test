#!/usr/bin/env bash
# Hetzner routed network for FI hypervisor — fixes guest MAC on uplink.
# NL is not modified. Passwords from infra/docker/.env.probe only.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT/infra/scripts/load-ops-env.sh"
load_ops_env "$ROOT"

: "${FI_SSH_PASS:?set FI_SSH_PASS in .env.probe}"
: "${NL_SSH_PASS:?set NL_SSH_PASS in .env.probe}"
FI_IP="${FI_IP:-95.216.1.155}"
NL_IP="${NL_IP:-66.248.206.14}"

run_fi() { ssh_hv "$FI_IP" "$FI_SSH_PASS" "$@"; }
run_nl() { ssh_hv "$NL_IP" "$NL_SSH_PASS" "$@"; }

echo "=== FI: migrate to libvirt routed (Hetzner) ==="
run_fi 'bash -s' <<'REMOTE'
set -euo pipefail
ROUTE_CIDR=135.181.123.152/29

virsh list --name | while read -r n; do
  [ -z "$n" ] && continue
  virsh shutdown "$n" --timeout 20 2>/dev/null || virsh destroy "$n" 2>/dev/null || true
done
sleep 3

if ip link show br0 type bridge >/dev/null 2>&1; then
  ip link set eth0 nomaster 2>/dev/null || true
  ip link set br0 down 2>/dev/null || true
  ip link del br0 2>/dev/null || true
fi

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

cp -a /etc/network/interfaces /etc/network/interfaces.bak.hetzner-routed
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

mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-hetzner-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=0
net.ipv4.conf.eth0.proxy_arp=0
net.ipv4.conf.br0.proxy_arp=0
SYS
sysctl -p /etc/sysctl.d/99-vf-hetzner-routed.conf

command -v ufw >/dev/null && ufw default allow routed 2>/dev/null || true

echo "Rebooting in 5s to apply network safely..."
sleep 5
nohup reboot >/dev/null 2>&1 &
REMOTE

echo "Waiting for FI reboot..."
for i in $(seq 1 36); do
  sleep 10
  if ssh_hv "$FI_IP" "$FI_SSH_PASS" true 2>/dev/null; then
    echo "FI back online after ${i}0s"
    break
  fi
done

run_fi 'ip route add 135.181.123.152/29 dev br0 2>/dev/null || true; ip -br addr; virsh net-list --all'

echo "=== VF DB: FI network + IP block gateway ==="
run_nl 'bash -s' <<'REMOTE'
set -euo pipefail
source /opt/virtfusion/app/control/.env
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" <<SQL
UPDATE hypervisor_networks SET type='libvirtRouted', bridge='br0', enabled=1 WHERE id=2 AND hypervisor_id=2;
UPDATE ip_blocks SET ipv4_gateway='135.181.123.1', ipv4_netmask='255.255.255.0' WHERE id=2;
SELECT id,type,bridge FROM hypervisor_networks WHERE hypervisor_id=2;
SELECT id,name,ipv4_gateway,ipv4_netmask FROM ip_blocks WHERE id=2;
SQL
REMOTE

run_fi 'supervisorctl restart vf-queue-hv: 2>/dev/null; systemctl restart libvirtd 2>/dev/null; true'
echo VF_FI_HETZNER_ROUTED_DONE
