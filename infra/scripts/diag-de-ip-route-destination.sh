#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
IP=212.102.227.40

SSHPASS="$DE_MID_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 \
  "timeout 8 tcpdump -ni br0 host $IP" > /tmp/mid-route-capture.txt 2>&1 &
MID_PID=$!

SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@185.84.224.84 \
  "timeout 8 tcpdump -ni br0 host $IP" > /tmp/prosto-route-capture.txt 2>&1 &
PROSTO_PID=$!

sleep 2
ping -c 3 -W 2 "$IP" || true
wait "$MID_PID" || true
wait "$PROSTO_PID" || true

echo "=== DE-mid capture ==="
python3 -c 'print(open("/tmp/mid-route-capture.txt", errors="replace").read())'
echo "=== DE-prosto capture ==="
python3 -c 'print(open("/tmp/prosto-route-capture.txt", errors="replace").read())'
