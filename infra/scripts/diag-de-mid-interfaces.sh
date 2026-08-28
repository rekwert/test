#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$DE_MID_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.151.40.165 bash -s <<'HV5'
ip -br link
echo "=== bridges ==="
bridge link
echo "=== interfaces config ==="
python3 -c 'print(open("/etc/network/interfaces").read())'
echo "=== MAC mapping ==="
for path in /sys/class/net/*; do
  printf '%s ' "${path##*/}"
  python3 -c "print(open('$path/address').read().strip())"
done
HV5
