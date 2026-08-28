#!/usr/bin/env bash
# OpenStack Sunbeam on Ubuntu 22.04 bare metal (control plane only).
# Run as root on the NEW control server — not on compute nodes with VF clients.
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y snapd curl
systemctl enable --now snapd.socket
sleep 5

echo "Installing OpenStack snap..."
snap install openstack --channel=2024.1/stable || snap install openstack

echo "Preparing node..."
sunbeam prepare-node-script | bash -x
export PATH="/snap/bin:$PATH"

echo "Bootstrap cluster (control)..."
sunbeam cluster bootstrap --accept-defaults

echo "Demo OpenRC..."
sunbeam configure --accept-defaults --openrc /root/demo-openrc
# shellcheck disable=SC1091
source /root/demo-openrc

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/bootstrap-dev.sh" | tee /root/openstack-bootstrap.log

echo ""
echo "Control install done."
echo "  source /root/demo-openrc"
echo "  openstack hypervisor list"
echo "  openstack network list"
echo "Env for NL back VPS: /root/openstack-portal-dev.env (from bootstrap-dev.sh)"
