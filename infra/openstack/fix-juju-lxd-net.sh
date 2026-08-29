#!/usr/bin/env bash
# Fix LXD juju controller container networking (DHCP/DNS) during bootstrap.
set -euo pipefail
[[ "$(id -u)" -eq 0 ]] || exit 1

SUBNET="$(sudo -u sunbeam lxc network get lxdbr0 ipv4.address 2>/dev/null || true)"
GW="${SUBNET%/*}.1"
IP="${SUBNET%/*}.50"
CONTAINER="$(sudo -u sunbeam lxc list -c n --format csv 2>/dev/null | grep '^juju-' | head -1 || true)"

if [[ -z "$CONTAINER" ]]; then
  echo "No juju container yet"
  exit 0
fi

echo "Fixing $CONTAINER -> $IP gw $GW"
sudo -u sunbeam lxc exec "$CONTAINER" -- ip addr add "${IP}/24" dev eth0 2>/dev/null || true
sudo -u sunbeam lxc exec "$CONTAINER" -- ip link set eth0 up 2>/dev/null || true
sudo -u sunbeam lxc exec "$CONTAINER" -- ip route replace default via "$GW" 2>/dev/null || true
sudo -u sunbeam lxc exec "$CONTAINER" -- sh -c 'printf "nameserver 8.8.8.8\nnameserver 1.1.1.1\n" > /etc/resolv.conf' 2>/dev/null || true
sudo -u sunbeam lxc list "$CONTAINER"
