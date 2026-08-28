#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

SSHPASS="$DE_MID_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'HV5'
set +e
VM=$(virsh list --name | python3 -c 'import sys; print(next((x.strip() for x in sys.stdin if x.strip()), ""))')
echo "vm=$VM"
echo "=== host routes ==="
ip route show
ip route get 212.102.227.40
echo "=== bridge addresses ==="
ip -br addr show br0
echo "=== VM interface ==="
virsh domiflist "$VM"
virsh domifaddr "$VM" --source agent
echo "=== guest agent network ==="
virsh qemu-agent-command "$VM" '{"execute":"guest-network-get-interfaces"}'
echo
echo "=== direct host to VM ==="
ping -c 3 -W 2 212.102.227.40
ip neigh show dev br0
echo "=== forwarding ==="
sysctl net.ipv4.ip_forward net.ipv4.conf.all.proxy_arp net.ipv4.conf.br0.proxy_arp
iptables -L FORWARD -n -v
HV5

echo "=== backend route to VM ==="
ip route get 212.102.227.40
ping -c 2 -W 2 212.102.227.40 || true
