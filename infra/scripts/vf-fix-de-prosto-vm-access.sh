#!/usr/bin/env bash
# Re-attach all VM tap interfaces to br0 on DE-prosto (no VM reboot, no HV reboot).
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe

DE_IP=212.102.227.6

echo "=== Before: taps on br0 ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" \
  'bridge link show 2>/dev/null | wc -l; virsh list --name | wc -l'

echo "=== Attach orphan taps to br0 ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" 'bash -s' <<'HV'
set -euo pipefail
attached=0
skipped=0
for vm in $(virsh list --name); do
  [ -z "$vm" ] && continue
  tap=$(virsh dumpxml "$vm" 2>/dev/null | grep -oP "target dev='\K[0-9]+" | head -1)
  [ -z "$tap" ] && continue
  if ip link show "$tap" 2>/dev/null | grep -q 'master br0'; then
    skipped=$((skipped+1))
    continue
  fi
  if ip link show "$tap" >/dev/null 2>&1; then
    ip link set "$tap" master br0 2>/dev/null && echo "attached $tap ($vm)" && attached=$((attached+1)) || echo "FAIL $tap ($vm)"
  fi
done
echo "attached=$attached skipped_on_br0=$skipped"
echo "=== bridge link count ==="
bridge link show 2>/dev/null | wc -l
HV

echo "=== NL: nat sync + network configs ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'cd /opt/virtfusion/app/control && /opt/virtfusion/php8/bin/php artisan nat:sync-hypervisor 3 >/dev/null; vfcli-ctrl server:configurations 3 --options=network -n 2>&1 | tail -3'

echo "=== Pool gateway + reserve .6/.7 + fix 551/553 ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" "bash -s" <<'POOL'
set -euo pipefail
GW=212.102.227.1; CIDR=212.102.227.0/24
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
ip route replace ${CIDR} dev br0 scope link 2>/dev/null || true
ip addr show dev br0 | grep -q "${GW}/" || ip addr add ${GW}/32 dev br0
grep -q vf-routed-pool /etc/network/interfaces 2>/dev/null || cat >>/etc/network/interfaces <<IFACE

# vf-routed-pool
up ip route replace ${CIDR} dev br0 scope link || true
up ip addr add ${GW}/32 dev br0 || true
IFACE
POOL

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 'bash -s' <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" <<'SQL'
UPDATE ipv4 SET server_id=NULL, reserved=1 WHERE address IN (INET_ATON('212.102.227.6'), INET_ATON('212.102.227.7'));
UPDATE ipv4 SET server_id=NULL, interface_id=NULL WHERE server_id IN (551,553);
UPDATE ipv4 SET server_id=551, interface_id=551 WHERE address=INET_ATON('212.102.227.41');
UPDATE ipv4 SET server_id=553, interface_id=553 WHERE address=INET_ATON('212.102.227.42');
SELECT s.id,s.name,INET_NTOA(v4.address) ip FROM servers s JOIN ipv4 v4 ON v4.server_id=s.id WHERE s.id IN (551,553);
SQL
NL

SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" bash -s <<'GA'
for spec in c853c4bd-6960-45ed-ac26-37ba8d22bb1f:212.102.227.41 40cca3ad-1933-4401-a75e-8d3100c1012b:212.102.227.42; do
  vm=${spec%%:*}; ip=${spec##*:}
  virsh qemu-agent-command "$vm" "{\"execute\":\"guest-exec\",\"arguments\":{\"path\":\"/bin/bash\",\"arg\":[\"-c\",\"ip addr flush dev ens3; ip addr add ${ip}/24 dev ens3; ip route replace default via 212.102.227.1 dev ens3\"],\"capture-output\":true}}" >/dev/null 2>&1 || true
done
GA

echo "=== Verify ping from NL ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'for ip in 212.102.227.3 212.102.227.4 212.102.227.10 212.102.227.39 212.102.227.40; do ping -c1 -W2 $ip >/dev/null 2>&1 && echo $ip OK || echo $ip FAIL; done'

echo "=== Verify SSH sample ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  'for ip in 212.102.227.3 212.102.227.4 212.102.227.39; do nc -zvw3 $ip 22 >/dev/null 2>&1 && echo $ip:22 OPEN || echo $ip:22 closed; done'

echo "DE_PROSTO_VM_ACCESS_FIXED"
