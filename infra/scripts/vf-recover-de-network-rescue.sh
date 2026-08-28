#!/usr/bin/env bash
# Recover DE-prosto network from Hostkey rescue mode (when br0 migration dropped SSH).
# Run ON THE DE SERVER while booted in Hostkey rescue.
#
# Usage (in rescue shell as root):
#   curl -fsSL .../vf-recover-de-network-rescue.sh | bash
#   # or after copying:
#   bash vf-recover-de-network-rescue.sh
#
# Then reboot into normal mode from Hostkey panel.
set -euo pipefail

ROOTPW="${1:-}"
ROOT_DEV="${ROOT_DEV:-/dev/md2}"
BOOT_DEV="${BOOT_DEV:-/dev/md1}"

mount_root() {
  mkdir -p /mnt/sys
  mountpoint -q /mnt/sys || mount "$ROOT_DEV" /mnt/sys
  mountpoint -q /mnt/sys/boot || mount "$BOOT_DEV" /mnt/sys/boot 2>/dev/null || true
  for d in dev proc sys; do
    mountpoint -q "/mnt/sys/$d" || mount --bind "/$d" "/mnt/sys/$d"
  done
}

echo "=== Mount installed system ==="
mount_root
echo "hostname=$(cat /mnt/sys/etc/hostname)"
head -3 /mnt/sys/etc/os-release

if [[ -n "$ROOTPW" ]]; then
  chroot /mnt/sys /bin/bash -c "echo 'root:${ROOTPW}' | chpasswd"
  echo "root password updated"
fi

mkdir -p /mnt/sys/etc/ssh/sshd_config.d
cat >/mnt/sys/etc/ssh/sshd_config.d/99-bootstrap.conf <<'EOF'
PermitRootLogin yes
PasswordAuthentication yes
EOF

cp -a /mnt/sys/etc/network/interfaces "/mnt/sys/etc/network/interfaces.bak.rescue.$(date +%s)" 2>/dev/null || true

# Phase 1: restore SSH with public IP on eno1 (no bridge yet).
cat >/mnt/sys/etc/network/interfaces <<'EOF'
auto lo
iface lo inet loopback

auto eno1
iface eno1 inet static
    address 185.84.224.84
    netmask 255.255.255.192
    gateway 185.84.224.65
    dns-nameservers 1.1.1.1 8.8.8.8
EOF

echo "=== New /etc/network/interfaces ==="
cat /mnt/sys/etc/network/interfaces

umount /mnt/sys/sys /mnt/sys/proc /mnt/sys/dev 2>/dev/null || true
umount /mnt/sys/boot 2>/dev/null || true
umount /mnt/sys

echo ""
echo "RECOVERY_WRITTEN"
echo "Reboot the server to normal mode from Hostkey panel, then verify:"
echo "  ssh root@185.84.224.84"
echo "After SSH works, run br0 + VirtFusion setup from back (vf-setup-de-hypervisor-host.sh phase 2)."
