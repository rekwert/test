#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
FI_PASS="${FI_PASS:?missing FI_PASS}"

check_host() {
  local label="$1" ip="$2" pass="$3"
  echo "=== $label $ip ==="
  SSHPASS="$pass" sshpass -e ssh -n -o StrictHostKeyChecking=no -o ConnectTimeout=15 root@"$ip" bash -c '
    echo "hostname=$(hostname)"
    echo "os=$(. /etc/os-release; echo "$PRETTY_NAME")"
    echo "kvm=$(test -e /dev/kvm && echo yes || echo no)"
    echo "vf_agent=$(test -d /opt/virtfusion/app/hypervisor && echo yes || echo no)"
    echo "libvirtd=$(systemctl is-active libvirtd 2>/dev/null || true)"
    echo "vf_nginx=$(systemctl is-active vf-nginx 2>/dev/null || true)"
    echo "port8892=$(ss -tln | python3 -c '\''import sys; print(sum(":8892" in x for x in sys.stdin))'\'')"
    ip -br addr
    ip route
    virsh list --all 2>/dev/null || true
  '
}

check_host "DE-prosto" "185.84.224.84" "$DE_SSH_PASS"
check_host "FI-node1" "95.216.1.155" "$FI_PASS"

echo "=== VirtFusion control state ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "SELECT id,name,ip,port,ssh_port,commissioned,enabled,maintenance,prohibit,LENGTH(token) token_len,hypervisor_group_id FROM hypervisors WHERE id IN (2,3);"
echo "=== latest monitor rows ==="
"${MY[@]}" -e "SELECT * FROM sys_mon_hypervisor WHERE hypervisor_id IN (2,3) ORDER BY id DESC LIMIT 6;"
echo "=== NL SSH direct ==="
for spec in "2 95.216.1.155" "3 185.84.224.84"; do
  set -- $spec
  printf "HV%s " "$1"
  ssh -n -o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=no root@"$2" hostname 2>&1 || true
done
NL
