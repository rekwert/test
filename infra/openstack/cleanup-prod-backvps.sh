#!/usr/bin/env bash
# Remove Sunbeam/OpenStack install artifacts from production backvps.
# Safe to run on backvps only — does NOT touch docker compose stack.
set -euo pipefail
[[ "$(id -u)" -eq 0 ]] || { echo "Run as root"; exit 1; }

echo "=== Stop Sunbeam install processes ==="
pkill -f sunbeam-resume 2>/dev/null || true
pkill -f continue-sunbeam 2>/dev/null || true
pkill -f juju-bootstrap 2>/dev/null || true
pkill -f sunbeam-prepare 2>/dev/null || true

echo "=== Ensure portal gateway is up ==="
if command -v docker >/dev/null 2>&1; then
  docker start docker-gateway-1 2>/dev/null || true
fi

echo "=== Remove Sunbeam snaps (if present) ==="
for s in openstack juju lxd; do
  if snap list "$s" >/dev/null 2>&1; then
    snap stop "$s" 2>/dev/null || true
    snap remove --purge "$s" || true
  fi
done

echo "=== Remove sunbeam user ==="
if id sunbeam >/dev/null 2>&1; then
  userdel -r sunbeam 2>/dev/null || userdel sunbeam 2>/dev/null || true
fi
rm -f /etc/sudoers.d/90-sunbeam /etc/sudoers.d/90-sunbeam-sunbeam

echo "=== Cleanup network bridge ==="
ip link del sunbeambr0 2>/dev/null || true

echo "=== Remove install logs/scripts ==="
rm -f /root/sunbeam-*.log /root/continue-sunbeam-install.sh /root/sunbeam-resume.sh /root/juju-bootstrap.sh
rm -f /root/.ssh/id_ed25519_sunbeam_local /root/.ssh/id_ed25519_sunbeam_local.pub

# Keep /opt/openstack-portal only if it is not the prod tree
if [[ -d /opt/openstack-portal ]] && [[ ! -d /opt/testVPStrade ]]; then
  rm -rf /opt/openstack-portal
fi

echo "=== Docker stack status ==="
docker ps --format 'table {{.Names}}\t{{.Status}}' | head -15 || true
HEALTH=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 https://cloud-hustle.com/api/v1/health || echo fail)
echo "health: $HEALTH"
echo "=== Production cleanup done ==="
