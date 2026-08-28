#!/usr/bin/env bash
# OpenStack MicroStack snap on Ubuntu 22.04 (jammy) — Sunbeam requires 24.04 (noble).
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq snapd curl
systemctl enable --now snapd.socket
sleep 5

echo "Removing Sunbeam openstack snap if present (jammy incompatible)..."
snap remove openstack 2>/dev/null || true

echo "Installing MicroStack snap (beta, devmode — required on 22.04)..."
snap install microstack --beta --devmode

echo "Initializing MicroStack control node (10-25 min)..."
microstack init --auto --control

OPENRC="/var/snap/microstack/current/etc/openrc"
if [[ -f "$OPENRC" ]]; then
  cp "$OPENRC" /root/demo-openrc
fi

PASS="$(snap get microstack config.credentials.keystone-password 2>/dev/null | awk '{print $2}' || true)"
if [[ -n "$PASS" ]]; then
  echo "Keystone admin password: $PASS"
  echo "$PASS" > /root/.openstack-admin-pass
  chmod 600 /root/.openstack-admin-pass
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENSTACK_CLI=microstack.openstack bash "$SCRIPT_DIR/bootstrap-dev.sh" | tee /root/openstack-bootstrap.log

if [[ -n "$PASS" ]] && [[ -f /root/openstack-portal-dev.env ]]; then
  sed -i "s/OPENSTACK_PASSWORD=CHANGE_ME/OPENSTACK_PASSWORD=${PASS}/" /root/openstack-portal-dev.env
fi

echo ""
echo "MicroStack control install done (Ubuntu 22.04)."
echo "  source /root/demo-openrc   OR   microstack.openstack ..."
echo "  cat /root/openstack-portal-dev.env"
echo "  microstack.openstack server list"
