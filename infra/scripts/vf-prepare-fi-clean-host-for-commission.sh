#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
FI_PASS="${FI_PASS:?missing FI_PASS}"

echo "=== Prepare clean FI host without changing its uplink ==="
export SSHPASS="$FI_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no root@95.216.1.155 bash -s <<'FI'
set -euo pipefail
hostnamectl set-hostname FI-node1
mkdir -p /home/vf-data/disk
chmod 755 /home/vf-data /home/vf-data/disk
cat >/etc/sysctl.d/99-vf-hetzner-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=0
net.ipv4.conf.eno1.proxy_arp=0
net.ipv4.conf.br0.proxy_arp=0
SYS
sysctl -p /etc/sysctl.d/99-vf-hetzner-routed.conf
virsh net-autostart br0
virsh net-start br0 2>/dev/null || true
ip route replace 135.181.123.152/29 dev br0
echo "host=$(hostname)"
ip -br addr
ip route
virsh net-list --all
FI

echo "=== Authorize VirtFusion control SSH key and reset only HV2 auth ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "FI_PASS='$FI_PASS' bash -s" <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
install -d -m 700 /root/.ssh
if [[ ! -f /root/.ssh/id_ed25519 ]]; then
  ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519
fi
PUB=$(< /root/.ssh/id_ed25519.pub)
ssh-keygen -f /root/.ssh/known_hosts -R 95.216.1.155 2>/dev/null || true
export SSHPASS="$FI_PASS"
sshpass -e ssh -n -o StrictHostKeyChecking=no root@95.216.1.155 \
  "install -d -m 700 /root/.ssh; touch /root/.ssh/authorized_keys; grep -qxF '$PUB' /root/.ssh/authorized_keys || echo '$PUB' >> /root/.ssh/authorized_keys; chmod 600 /root/.ssh/authorized_keys"
ssh -n -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=no root@95.216.1.155 hostname
"${MY[@]}" -e "UPDATE hypervisors SET commissioned=0, token=NULL, enabled=1, maintenance=0, prohibit=0 WHERE id=2;"
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) token_len FROM hypervisors WHERE id=2;"
echo FI_READY_FOR_OFFICIAL_COMMISSION
NL
