#!/usr/bin/env bash
# Wipe VirtFusion and reinstall control+hypervisor on same host (Debian 12).
# Preserves /etc/network/interfaces (br0).
set -euo pipefail

LOG=/root/virtfusion-reinstall-$(date +%Y%m%d-%H%M%S).log
exec > >(tee -a "$LOG") 2>&1

echo "=== VF FULL REINSTALL started $(date -Is) ==="
export DEBIAN_FRONTEND=noninteractive

echo "=== 1. Stop services ==="
supervisorctl shutdown 2>/dev/null || true
for u in vf-nginx vf-php8-fpm vf-control-wss vf-system-php8-fpm; do
  systemctl stop "$u" 2>/dev/null || true
done
sleep 2

echo "=== 2. Backup network ==="
cp -a /etc/network/interfaces "/etc/network/interfaces.bak.$(date +%s)" 2>/dev/null || true

echo "=== 3. Drop old MariaDB VF databases ==="
for db in $(mysql -N -e "SHOW DATABASES LIKE 'vf_%';" 2>/dev/null || true); do
  echo "DROP DATABASE $db"
  mysql -e "DROP DATABASE IF EXISTS \`$db\`;" 2>/dev/null || true
done

echo "=== 4. Remove data ==="
rm -rf /opt/virtfusion
rm -rf /home/vf-data
mkdir -p /home/vf-data/disk
chmod 755 /home/vf-data /home/vf-data/disk

echo "=== 5. Purge VF packages (if installed) ==="
apt-get remove --purge -y vf-control-wss vf-nginx vf-php8 vf-system-php8 2>/dev/null || true
apt-get autoremove -y 2>/dev/null || true

echo "=== 6. Ensure deps ==="
apt-get update -y
apt-get install -y curl bridge-utils ifupdown2 lxc debootstrap mariadb-server 2>/dev/null || \
apt-get install -y curl bridge-utils ifupdown2 lxc debootstrap 2>/dev/null || true

echo "=== 7. Hypervisor install ==="
( set -euo pipefail
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
)

echo "=== 8. Control server install ==="
curl -fsSL https://install.virtfusion.net/install-control-debian-12.sh | sh -s -- --verbose

echo "=== 9. Start libvirt ==="
systemctl enable libvirtd 2>/dev/null || true
systemctl start libvirtd 2>/dev/null || true

echo "=== 10. Health ==="
systemctl is-active vf-nginx vf-php8-fpm libvirtd 2>/dev/null || true
curl -sk -o /dev/null -w "login:%{http_code}\n" https://127.0.0.1/login || true

echo "=== REINSTALL COMPLETE $(date -Is) ==="
echo "Log: $LOG"
echo "Check installer output above for admin password."
