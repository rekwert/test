#!/usr/bin/env bash
set -euo pipefail
LOG=/root/virtfusion-install.log
exec > >(tee -a "$LOG") 2>&1

echo "=== VirtFusion install started $(date -Is) ==="
export DEBIAN_FRONTEND=noninteractive

apt-get update -y
apt-get install -y curl bridge-utils ifupdown2

echo "=== Hypervisor ==="
( set -euo pipefail
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
)

echo "=== Control server (Debian 12) ==="
curl -fsSL https://install.virtfusion.net/install-control-debian-12.sh | sh -s -- --verbose

echo "=== Done $(date -Is) ==="
systemctl is-active vf-nginx 2>/dev/null || true
curl -sk -o /dev/null -w "login_http:%{http_code}\n" https://127.0.0.1/login || true
