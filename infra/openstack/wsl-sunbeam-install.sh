#!/usr/bin/env bash
# Install OpenStack Sunbeam in WSL2 Ubuntu for local dev.
# Requirements: WSL2 Ubuntu, 16GB+ RAM assigned to WSL, nested virt optional (slow without KVM).
set -euo pipefail

if ! grep -qi microsoft /proc/version 2>/dev/null; then
  echo "Run this script inside WSL2 Ubuntu (not native Linux VM is also OK)."
fi

if ! command -v snap >/dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y snapd
  sudo systemctl enable --now snapd.socket
fi

echo "Installing OpenStack snap (Sunbeam)... this can take 10-20 minutes."
sudo snap install openstack --channel=2024.1/stable || sudo snap install openstack

echo "Preparing node..."
sudo sunbeam prepare-node-script | bash -x || true
newgrp snap_daemon <<'EOS' || true
sunbeam cluster bootstrap --accept-defaults -o bootstrap.log || sunbeam cluster bootstrap --accept-defaults
EOS

echo "Configuring demo OpenRC..."
sunbeam configure --accept-defaults --openrc ~/demo-openrc || true
# shellcheck disable=SC1090
source ~/demo-openrc 2>/dev/null || true

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/bootstrap-dev.sh"

echo ""
echo "OpenStack Sunbeam install finished."
echo "Source credentials: source ~/demo-openrc"
echo "Horizon/dashboard and Keystone are on localhost (WSL ports forward to Windows)."
echo "From Docker on Windows use OPENSTACK_AUTH_URL=http://host.docker.internal:5000/v3"
echo "Run: openstack server list"
