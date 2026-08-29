#!/usr/bin/env bash
# OpenStack Sunbeam install for DEDICATED control node (194.164.216.185).
# Run ONLY on the control bare metal — NOT on backvps/production.
# Usage (direct SSH as root on control):
#   curl -fsSL https://raw.githubusercontent.com/rekwert/test/main/infra/openstack/install-control-only.sh | bash
set -euo pipefail
[[ "$(id -u)" -eq 0 ]] || { echo "Run as root"; exit 1; }

# Safety: refuse if portal docker stack detected
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^docker-gateway-1$'; then
  echo "ERROR: docker-gateway-1 found — this looks like production backvps."
  echo "Run OpenStack install on dedicated control bare metal, not portal host."
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
REPO="${OPENSTACK_WORKDIR:-/opt/openstack-portal}"
LOG="/root/openstack-control-install.log"
exec > >(tee -a "$LOG") 2>&1

echo "=== $(date -Is) control-only OpenStack install ==="
echo "hostname: $(hostname -f)"
echo "MemTotal: $(grep MemTotal /proc/meminfo)"
echo "public IP: $(curl -s --max-time 5 ifconfig.me || echo unknown)"

apt-get update -qq
apt-get install -y -qq snapd curl git tmux systemd-container
systemctl enable --now snapd.socket
sleep 5

rm -rf "$REPO"
git clone --depth 1 "${OPENSTACK_REPO:-https://github.com/rekwert/test.git}" "$REPO"

# Sunbeam user + SSH localhost runner (snap cgroup safe)
useradd -m -s /bin/bash sunbeam 2>/dev/null || true
usermod -aG sudo sunbeam
echo 'sunbeam ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-sunbeam
chmod 440 /etc/sudoers.d/90-sunbeam
loginctl enable-linger sunbeam

KEY="/root/.ssh/id_ed25519_sunbeam_local"
if [[ ! -f "$KEY" ]]; then
  ssh-keygen -t ed25519 -N "" -f "$KEY"
fi
install -d -m 700 -o sunbeam -g sunbeam /home/sunbeam/.ssh
grep -qF "$(cat "${KEY}.pub")" /home/sunbeam/.ssh/authorized_keys 2>/dev/null || \
  cat "${KEY}.pub" >> /home/sunbeam/.ssh/authorized_keys
chown sunbeam:sunbeam /home/sunbeam/.ssh/authorized_keys
chmod 600 /home/sunbeam/.ssh/authorized_keys

run_as_sunbeam() {
  ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    sunbeam@127.0.0.1 "export PATH=/snap/bin:\$PATH; $*"
}

snap install openstack --channel=2024.1/stable || snap install openstack

echo "=== prepare-node ==="
run_as_sunbeam "bash $REPO/infra/openstack/sunbeam-prepare.sh"

echo "=== cluster bootstrap ==="
run_as_sunbeam "sunbeam cluster bootstrap --accept-defaults"

echo "=== configure ==="
run_as_sunbeam "sunbeam configure --accept-defaults --openrc ~/demo-openrc"
install -m 600 /home/sunbeam/demo-openrc /root/demo-openrc

# shellcheck disable=SC1091
source /root/demo-openrc
bash "$REPO/infra/openstack/bootstrap-dev.sh" | tee /root/openstack-bootstrap.log

openstack token issue
openstack network list
cat /root/openstack-portal-dev.env
echo "=== DONE $(date -Is) ==="
