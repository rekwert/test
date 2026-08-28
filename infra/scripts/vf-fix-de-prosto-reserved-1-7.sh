#!/usr/bin/env bash
# DE-prosto: reserve 212.102.227.1-7, reassign affected VMs, restore tap routing.
# No hypervisor reboot, no VM reboot (guest IP via qemu-guest-agent).
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env

DE_IP=212.102.227.6
POOL_GW=212.102.227.1
POOL_CIDR=212.102.227.0/24

echo "=== 1. Hypervisor: pool gw + reattach taps ==="
SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" bash -s <<REMOTE
set -euo pipefail
GW='$POOL_GW'; CIDR='$POOL_CIDR'
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
ip route replace \${CIDR} dev br0 scope link 2>/dev/null || true
ip addr show dev br0 | grep -q "\${GW}/" || ip addr add \${GW}/32 dev br0
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
command -v ufw >/dev/null && ufw default allow routed >/dev/null 2>&1 && ufw reload >/dev/null 2>&1 || true
attached=0
for vm in \$(virsh list --name); do
  [ -z "\$vm" ] && continue
  tap=\$(virsh dumpxml "\$vm" 2>/dev/null | grep -oP "target dev='\K[0-9]+" | head -1)
  [ -z "\$tap" ] && continue
  if ! ip link show "\$tap" 2>/dev/null | grep -q 'master br0'; then
    ip link set "\$tap" master br0 && attached=\$((attached+1))
  fi
done
grep -q vf-reattach-taps /etc/rc.local 2>/dev/null || cat >>/etc/rc.local <<'RC'

# vf-reattach-taps
for vm in \$(virsh list --name 2>/dev/null); do
  [ -z "\$vm" ] && continue
  tap=\$(virsh dumpxml "\$vm" 2>/dev/null | grep -oP "target dev='\K[0-9]+" | head -1)
  [ -z "\$tap" ] && continue
  ip link show "\$tap" 2>/dev/null | grep -q 'master br0' || ip link set "\$tap" master br0 2>/dev/null || true
done
RC
echo "reattached=\$attached running=\$(virsh list --state-running | grep -c running)"
REMOTE

echo "=== 2. VF DB: reserve .1-.7, reassign VMs ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" <<'SQL'
UPDATE ipv4 SET server_id=NULL, reserved=1
WHERE block_id=6 AND address BETWEEN INET_ATON('212.102.227.1') AND INET_ATON('212.102.227.7');
UPDATE ipv4 SET server_id=NULL, interface_id=NULL
WHERE block_id=6 AND address IN (INET_ATON('212.102.227.3'), INET_ATON('212.102.227.4'), INET_ATON('212.102.227.5'));
UPDATE ipv4 SET server_id=597, interface_id=597, reserved=0 WHERE address=INET_ATON('212.102.227.43') AND block_id=6;
UPDATE ipv4 SET server_id=599, interface_id=599, reserved=0 WHERE address=INET_ATON('212.102.227.44') AND block_id=6;
UPDATE ipv4 SET server_id=517, interface_id=517, reserved=0 WHERE address=INET_ATON('212.102.227.45') AND block_id=6;
UPDATE ipv4 SET server_id=551, interface_id=551, reserved=0 WHERE address=INET_ATON('212.102.227.41') AND block_id=6;
UPDATE ipv4 SET server_id=553, interface_id=553, reserved=0 WHERE address=INET_ATON('212.102.227.42') AND block_id=6;
SELECT INET_NTOA(address) ip, server_id, reserved FROM ipv4
WHERE block_id=6 AND (address BETWEEN INET_ATON('212.102.227.1') AND INET_ATON('212.102.227.7')
   OR server_id IN (597,599,517,551,553)) ORDER BY address;
SQL
cd /opt/virtfusion/app/control
/opt/virtfusion/php8/bin/php artisan nat:sync-hypervisor 3 2>&1 | tail -2
vfcli-ctrl server:configurations 3 --options=network -n 2>&1 | tail -2
NL

echo "=== 3. Guest IPs (qemu-guest-agent) ==="
declare -A VM_IP=([597]=212.102.227.43 [599]=212.102.227.44 [517]=212.102.227.45 [551]=212.102.227.41 [553]=212.102.227.42)
UUID_MAP=$(SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N -e \
  "SELECT id,uuid FROM servers WHERE id IN (597,599,517,551,553);"
NL
)
APPLY_CMDS=""
while read -r sid uuid; do
  [ -z "$sid" ] || [ -z "$uuid" ] && continue
  ip="${VM_IP[$sid]:-}"
  [ -n "$ip" ] && APPLY_CMDS+="apply_ip $uuid $ip"$'\n'
done <<< "$UUID_MAP"

SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$DE_IP" bash -s <<HV
set -euo pipefail
GW=212.102.227.1
apply_ip() {
  local vm=\$1 ip=\$2
  if ! virsh domstate "\$vm" 2>/dev/null | grep -q running; then echo "skip \$vm not running"; return 0; fi
  if ! virsh qemu-agent-command "\$vm" '{"execute":"guest-ping"}' >/dev/null 2>&1; then echo "skip \$vm no guest-agent"; return 0; fi
  pid=\$(virsh qemu-agent-command "\$vm" "{\"execute\":\"guest-exec\",\"arguments\":{\"path\":\"/bin/bash\",\"arg\":[\"-c\",\"ip addr flush dev ens3 2>/dev/null; ip addr flush dev eth0 2>/dev/null; IF=ens3; ip link show ens3 >/dev/null 2>&1 || IF=eth0; ip addr add \${ip}/24 dev \\\$IF; ip route replace default via \${GW} dev \\\$IF; ip -br addr show \\\$IF\"],\"capture-output\":true}}" | python3 -c "import json,sys; print(json.load(sys.stdin)['return']['pid'])")
  sleep 2
  echo -n "\$vm -> \$ip: "
  virsh qemu-agent-command "\$vm" "{\"execute\":\"guest-exec-status\",\"arguments\":{\"pid\":\$pid}}" | python3 -c "import json,sys,base64; d=json.load(sys.stdin)['return']; print(base64.b64decode(d.get('out-data') or b'').decode().strip() or 'ok')"
}
$(printf '%s' "$APPLY_CMDS")
HV

echo "=== 4. Portal IP update ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.instances SET ip_address='212.102.227.43/32', updated_at=now() WHERE external_id='597';
UPDATE vps.instances SET ip_address='212.102.227.44/32', updated_at=now() WHERE external_id='599';
UPDATE vps.instances SET ip_address='212.102.227.45/32', updated_at=now() WHERE external_id='517';
UPDATE vps.instances SET ip_address='212.102.227.41/32', updated_at=now() WHERE external_id='551';
UPDATE vps.instances SET ip_address='212.102.227.42/32', updated_at=now() WHERE external_id='553';
SELECT hostname, external_id, ip_address::text, state FROM vps.instances
WHERE external_id IN ('597','599','517','551','553') ORDER BY external_id;
SQL

echo "=== 5. Verify from NL ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
for ip in 212.102.227.{1,2,3,4,5,6,7} 212.102.227.{41,42,43,44,45} 212.102.227.{10,39,40}; do
  ping -c1 -W1 $ip >/dev/null 2>&1 && echo "$ip OK" || echo "$ip FAIL"
done
NL

echo "DE_PROSTO_RESERVED_1_7_DONE"
