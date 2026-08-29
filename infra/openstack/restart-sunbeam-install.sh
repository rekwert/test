#!/usr/bin/env bash
set -euo pipefail
pkill -f continue-sunbeam-install 2>/dev/null || true
useradd -m -s /bin/bash sunbeam 2>/dev/null || true
usermod -aG sudo sunbeam
echo 'sunbeam ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-sunbeam
chmod 440 /etc/sudoers.d/90-sunbeam
loginctl enable-linger sunbeam 2>/dev/null || true
curl -fsSL https://raw.githubusercontent.com/rekwert/test/main/infra/openstack/continue-sunbeam-install.sh -o /root/continue-sunbeam-install.sh
: > /root/sunbeam-install-continue.log
nohup bash /root/continue-sunbeam-install.sh >> /root/sunbeam-install-continue.log 2>&1 &
echo "Started PID $! — tail -f /root/sunbeam-install-continue.log"
