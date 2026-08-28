#!/bin/bash
set -euo pipefail

echo "=== HOST ==="
hostname
uname -a | head -1
ip -br addr show br0 2>/dev/null || true
ip -br link show enp2s0f0 2>/dev/null || true

echo "=== TIME ==="
timedatectl | head -6

echo "=== LIBVIRT ==="
systemctl is-active libvirtd || true
virsh list --all 2>/dev/null | head -10 || true

echo "=== VF SERVICES ==="
supervisorctl status 2>/dev/null | grep -E 'vf-queue|vf-queue-hv|RUNNING|FATAL' || true

echo "=== LOG FILES ==="
find /opt/virtfusion /var/log -name '*.log' 2>/dev/null | head -40 || true

echo "=== EC 8 IN LOGS ==="
grep -ri 'EC 8\|Not valid\|resource alloc' /var/log/supervisor/ /opt/virtfusion/ 2>/dev/null | tail -30 || true

echo "=== QUEUE LOGS ==="
supervisorctl tail -4000 vf-queue:00 2>/dev/null | grep -iE 'error|valid|EC |accept|license|hypervisor|Not valid' | tail -50 || true

echo "=== HV QUEUE LOGS ==="
supervisorctl tail -4000 vf-queue-hv:00 2>/dev/null | grep -iE 'error|valid|EC |accept|license|hypervisor|Not valid' | tail -50 || true

echo "=== VFCLI ==="
command -v vfcli-ctrl || true
vfcli-ctrl license:status 2>&1 | head -20 || true

echo "=== MYSQL TABLES ==="
mysql virtfusion -e "SHOW TABLES LIKE '%log%';" 2>/dev/null || true
mysql virtfusion -e "SHOW TABLES LIKE '%alloc%';" 2>/dev/null || true
mysql virtfusion -e "SHOW TABLES LIKE '%hypervisor%';" 2>/dev/null || true
mysql virtfusion -e "SHOW TABLES LIKE '%package%';" 2>/dev/null || true
mysql virtfusion -e "SHOW TABLES LIKE '%licen%';" 2>/dev/null || true

echo "=== HYPERVISOR ROW ==="
mysql virtfusion -e "SELECT * FROM hypervisors WHERE id=1\G" 2>/dev/null | head -60 || true

echo "=== PACKAGE ROW ==="
mysql virtfusion -e "SELECT * FROM packages WHERE id=1\G" 2>/dev/null | head -60 || true

echo "=== RECENT RESOURCE LOG ROWS ==="
for t in resource_allocation_logs resource_allocation_log; do
  if mysql virtfusion -e "SELECT 1 FROM ${t} LIMIT 1" 2>/dev/null; then
    echo "-- table ${t} --"
    mysql virtfusion -e "SELECT * FROM ${t} ORDER BY id DESC LIMIT 5\G" 2>/dev/null || true
  fi
done

echo "=== LOCAL API ACCEPT ==="
curl -sk https://127.0.0.1/api/v1/compute/hypervisors/groups/1/resources 2>/dev/null | head -c 800 || true
echo
