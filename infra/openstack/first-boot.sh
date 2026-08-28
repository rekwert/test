#!/usr/bin/env bash
# One-shot OpenStack control install on fresh Ubuntu 22.04 bare metal.
# Usage (as root):
#   curl -fsSL https://raw.githubusercontent.com/rekwert/test/main/infra/openstack/first-boot.sh | bash
# Or after git clone:
#   bash infra/openstack/first-boot.sh
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
REPO="${OPENSTACK_REPO:-https://github.com/rekwert/test.git}"
WORKDIR="${OPENSTACK_WORKDIR:-/opt/openstack-portal}"

echo "=== first-boot: packages ==="
apt-get update -qq
apt-get install -y -qq git curl snapd ufw

echo "=== first-boot: clone ==="
rm -rf "$WORKDIR"
git clone --depth 1 "$REPO" "$WORKDIR"

echo "=== first-boot: firewall (SSH + OpenStack API) ==="
ufw allow OpenSSH || true
ufw allow 5000/tcp comment 'Keystone' || true
ufw allow 8774/tcp comment 'Nova' || true
ufw allow 9696/tcp comment 'Neutron' || true
ufw --force enable || true

echo "=== first-boot: OpenStack Sunbeam ==="
bash "$WORKDIR/infra/openstack/control-install.sh" 2>&1 | tee /root/openstack-install.log

echo ""
echo "=== DONE ==="
echo "OpenRC:  source /root/demo-openrc"
echo "Env:     cat /root/openstack-portal-dev.env"
echo "Log:     /root/openstack-install.log"
echo "Test:    openstack token issue && openstack network list"
