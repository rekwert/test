#!/bin/bash
echo "=== OS ==="
cat /etc/os-release | head -6
echo "=== DISK ==="
df -h / /home/vf-data 2>/dev/null
echo "=== VF INSTALLED ==="
ls -d /opt/virtfusion 2>/dev/null && du -sh /opt/virtfusion 2>/dev/null
echo "=== NETWORK ==="
ip -br addr show br0 enp2s0f0 2>/dev/null
brctl show br0 2>/dev/null || ip link show br0
