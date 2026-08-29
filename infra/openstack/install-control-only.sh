#!/usr/bin/env bash
# OpenStack Sunbeam install for DEDICATED control node (194.164.216.185).
# Run ONLY on the control bare metal — NOT on backvps/production.
# Usage (direct SSH as root on control):
#   curl -fsSL https://raw.githubusercontent.com/rekwert/test/main/infra/openstack/install-control-only.sh | bash
set -euo pipefail
[[ "$(id -u)" -eq 0 ]] || { echo "Run as root"; exit 1; }

# Stop leftover docker on dedicated control (frees ports 8443/5000 and RAM).
if command -v docker >/dev/null 2>&1 && docker ps -q 2>/dev/null | grep -q .; then
  echo "Stopping docker containers on control node..."
  docker stop $(docker ps -q) 2>/dev/null || true
fi

export DEBIAN_FRONTEND=noninteractive
REPO="${OPENSTACK_WORKDIR:-/opt/openstack-portal}"
LOG="/root/openstack-control-install.log"
exec > >(tee -a "$LOG") 2>&1

echo "=== $(date -Is) control-only OpenStack install ==="
echo "hostname: $(hostname -f)"
echo "MemTotal: $(grep MemTotal /proc/meminfo)"
echo "public IP: $(curl -s --max-time 5 ifconfig.me || echo unknown)"

# Clean previous partial Sunbeam attempts.
for s in openstack juju lxd; do
  snap remove --purge "$s" 2>/dev/null || true
done
pkill -u sunbeam 2>/dev/null || true
userdel -r sunbeam 2>/dev/null || true
rm -rf /home/sunbeam /var/snap/lxd /var/snap/openstack /var/snap/juju

sysctl -w net.ipv4.ip_forward=1
echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-ipforward.conf
sysctl -p /etc/sysctl.d/99-ipforward.conf || true

apt-get update -qq
apt-get install -y -qq snapd curl git tmux systemd-container
systemctl enable --now snapd.socket
sleep 5

rm -rf "$REPO"
git clone --depth 1 "${OPENSTACK_REPO:-https://github.com/rekwert/test.git}" "$REPO"

# Long-running install — avoid SSH timeout killing juju bootstrap.
export JUJU_CONTROLLER_AGENT_TIMEOUT=1800
export JUJU_BOOTSTRAP_TIMEOUT=1800

# Sunbeam user + SSH localhost runner (snap cgroup safe)
if id sunbeam >/dev/null 2>&1; then
  pkill -u sunbeam 2>/dev/null || true
  userdel -r sunbeam 2>/dev/null || true
fi
useradd -m -s /bin/bash sunbeam
usermod -aG sudo sunbeam
chown -R sunbeam:sunbeam /home/sunbeam
chmod 755 /home/sunbeam
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
