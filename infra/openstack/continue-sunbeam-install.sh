#!/usr/bin/env bash
# Continue Sunbeam install on control node (run as root).
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export PATH="/snap/bin:${PATH}"

REPO="${OPENSTACK_WORKDIR:-/opt/openstack-portal}"
SUNBEAM_USER="${SUNBEAM_USER:-sunbeam}"
LOG="/root/sunbeam-install-continue.log"
exec > >(tee -a "$LOG") 2>&1

echo "=== $(date -Is) continue Sunbeam install ==="

# OpenStack snap (Sunbeam).
if ! snap list openstack >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq snapd curl git tmux
  systemctl enable --now snapd.socket
  sleep 5
  snap install openstack --channel=2024.1/stable || snap install openstack
fi

if [[ ! -d "$REPO/.git" ]]; then
  git clone --depth 1 "${OPENSTACK_REPO:-https://github.com/rekwert/test.git}" "$REPO"
fi

cd "$REPO"
git pull -q

# Sunbeam requires a non-root user with sudo.
if ! id "$SUNBEAM_USER" &>/dev/null; then
  useradd -m -s /bin/bash "$SUNBEAM_USER"
  usermod -aG sudo "$SUNBEAM_USER"
  echo "${SUNBEAM_USER} ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/90-${SUNBEAM_USER}-sunbeam"
  chmod 440 "/etc/sudoers.d/90-${SUNBEAM_USER}-sunbeam"
fi
loginctl enable-linger "$SUNBEAM_USER" 2>/dev/null || true

# Ensure machinectl fallback: localhost SSH key for sunbeam.
KEY="/root/.ssh/id_ed25519_sunbeam_local"
if [[ ! -f "$KEY" ]]; then
  ssh-keygen -t ed25519 -N "" -f "$KEY"
fi
install -d -m 700 -o "$SUNBEAM_USER" -g "$SUNBEAM_USER" "/home/$SUNBEAM_USER/.ssh"
AUTH="/home/$SUNBEAM_USER/.ssh/authorized_keys"
grep -qF "$(cat "${KEY}.pub")" "$AUTH" 2>/dev/null || cat "${KEY}.pub" >> "$AUTH"
chown "$SUNBEAM_USER:$SUNBEAM_USER" "$AUTH"
chmod 600 "$AUTH"

run_as_sunbeam() {
  ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "${SUNBEAM_USER}@127.0.0.1" "export PATH=/snap/bin:\$PATH; $*"
}

echo "=== prepare-node ==="
if id -nG "$SUNBEAM_USER" | grep -qw snap_daemon; then
  echo "sunbeam already in snap_daemon, skip prepare"
else
  run_as_sunbeam "bash $REPO/infra/openstack/sunbeam-prepare.sh"
fi

echo "=== groups after prepare ==="
id "$SUNBEAM_USER"

echo "=== cluster bootstrap ==="
if [[ -f /home/$SUNBEAM_USER/demo-openrc ]] || [[ -f /root/demo-openrc ]]; then
  echo "demo-openrc exists, skip bootstrap"
else
  run_as_sunbeam "sunbeam cluster bootstrap --accept-defaults"
fi

echo "=== configure ==="
if [[ ! -f /home/$SUNBEAM_USER/demo-openrc ]]; then
  run_as_sunbeam "sunbeam configure --accept-defaults --openrc ~/demo-openrc"
fi
install -m 600 "/home/$SUNBEAM_USER/demo-openrc" /root/demo-openrc

echo "=== portal bootstrap ==="
# shellcheck disable=SC1091
source /root/demo-openrc
bash "$REPO/infra/openstack/bootstrap-dev.sh" | tee /root/openstack-bootstrap.log

echo "=== verify ==="
openstack token issue
openstack network list
echo "=== DONE $(date -Is) ==="
cat /root/openstack-portal-dev.env
