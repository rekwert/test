#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

echo "=== sys_mon / hypervisor monitor ==="
"${MY[@]}" -e "SELECT * FROM sys_mon_hypervisor WHERE hypervisor_id IN (3,5)\G" 2>/dev/null || \
"${MY[@]}" -e "SHOW TABLES LIKE '%mon%';"

echo "=== compare HV3 vs HV5 ==="
for IP in 185.84.224.84 66.151.40.165; do
  echo "--- $IP ---"
  ssh -o BatchMode=yes -o ConnectTimeout=10 root@$IP bash -s <<'H' || echo unreachable
echo host=$(hostname)
systemctl is-active libvirtd vf-nginx nginx 2>/dev/null | paste -sd' '
ss -tlnp 2>/dev/null | grep -E ':8892|:443|:80' || netstat -tlnp 2>/dev/null | grep 8892
test -d /opt/virtfusion/app/hypervisor && echo agent=ok || echo agent=missing
test -f /opt/virtfusion/app/hypervisor/conf/auth.json && echo auth=ok || echo auth=missing
supervisorctl status 2>/dev/null | head -6
ufw status 2>/dev/null | head -5 || iptables -L INPUT -n 2>/dev/null | head -8
H
done

echo "=== latest failed_jobs build ==="
"${MY[@]}" -e "SELECT id,LEFT(exception,300),failed_at FROM failed_jobs WHERE exception LIKE '%connectivity%' ORDER BY id DESC LIMIT 3\G"

echo "=== fix DE-mid: start vf-nginx, open 8892 ==="
ssh -o BatchMode=yes root@66.151.40.165 bash -s <<'FIX'
set -e
systemctl enable libvirtd
systemctl start libvirtd
# VF hypervisor API
if systemctl list-unit-files vf-nginx.service 2>/dev/null | grep -q vf-nginx; then
  systemctl enable vf-nginx
  systemctl start vf-nginx
fi
if ! ss -tlnp | grep -q ':8892'; then
  supervisorctl restart vf-queue-hv: 2>/dev/null || true
  sleep 3
fi
# Allow NL panel to reach hypervisor API
iptables -C INPUT -p tcp -s 66.248.206.14 --dport 8892 -j ACCEPT 2>/dev/null || \
  iptables -I INPUT 1 -p tcp -s 66.248.206.14 --dport 8892 -j ACCEPT
iptables -C INPUT -p tcp --dport 8892 -j ACCEPT 2>/dev/null || \
  iptables -I INPUT 2 -p tcp --dport 8892 -j ACCEPT
grep -q '8892' /etc/rc.local 2>/dev/null || {
  echo 'iptables -I INPUT 1 -p tcp -s 66.248.206.14 --dport 8892 -j ACCEPT 2>/dev/null || true' >> /etc/rc.local
}
echo after: $(ss -tlnp | grep 8892 || echo NO_8892)
FIX

echo "=== NL nc/curl HV5 ==="
nc -zv -w5 66.151.40.165 8892 2>&1
TOK=$(ssh -o BatchMode=yes root@66.151.40.165 "python3 -c \"import json;print(json.load(open('/opt/virtfusion/app/hypervisor/conf/auth.json'))['token'])\"" )
curl -sk -m 10 "https://66.151.40.165:8892/hypervisor/resources" -H "Authorization: Bearer $TOK" -w "\nRES=%{http_code}\n" | tail -5

supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL
