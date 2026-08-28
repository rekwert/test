#!/bin/bash
echo "=== VF PACKAGES ==="
dpkg -l | grep -i vf || true
echo "=== SUPERVISOR ==="
ls /etc/supervisor/conf.d/ 2>/dev/null
echo "=== SYSTEMD VF ==="
systemctl list-unit-files 'vf-*' --no-pager 2>/dev/null | head -20
