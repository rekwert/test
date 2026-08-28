#!/bin/bash
set -euo pipefail

echo "=== BEFORE ==="
systemctl is-active libvirtd 2>/dev/null || echo inactive
modprobe kvm 2>/dev/null || true
modprobe kvm_intel 2>/dev/null || modprobe kvm_amd 2>/dev/null || true

echo "=== START LIBVIRT ==="
systemctl enable libvirtd
systemctl start libvirtd
sleep 2
systemctl is-active libvirtd
virt-host-validate qemu 2>/dev/null | head -15 || true

echo "=== RESTART VF QUEUES ==="
supervisorctl restart vf-queue-hv: vf-queue: 2>/dev/null | tail -5

echo "=== NAT SYNC ==="
/opt/virtfusion/php8/bin/php /opt/virtfusion/app/control/artisan nat:sync-hypervisor 1 2>&1 | tail -3 || true

sleep 8
echo "=== AFTER ==="
systemctl status libvirtd --no-pager | head -8
virsh list --all 2>/dev/null | head -5
