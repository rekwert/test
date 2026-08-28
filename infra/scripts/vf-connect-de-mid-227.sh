#!/usr/bin/env bash
# Connect DE-mid (212.102.227.7) for midrange provisioning after network restore.
set -euo pipefail
ROOT="${ROOT:-/opt/testVPStrade}"
set -a
source "$ROOT/infra/docker/.env"
source "$ROOT/infra/docker/.env.probe"
set +a

MID_IP=212.102.227.7
MID_PASS='xV1bFQjD7-'
POOL_GW=212.102.227.1

echo "=== 1. Plan map + midrange packages ==="
bash "$ROOT/infra/scripts/ensure-vf-plan-map.sh" "$ROOT/infra/docker/.env"
SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no \
  "$ROOT/infra/scripts/vf-fix-midrange-packages-all.sh" root@66.248.206.14:/tmp/
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "sed -i 's/\r$//' /tmp/vf-fix-midrange-packages-all.sh && bash /tmp/vf-fix-midrange-packages-all.sh"

echo "=== 2. DE-mid host: pool routing (no reboot) ==="
SSHPASS="$MID_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID_IP" \
  "POOL_GW='$POOL_GW' NODE_IP='$MID_IP' bash -s" <<'MID'
set -euo pipefail
if ! ip link show br0 >/dev/null 2>&1; then
  IF=$(ip -o link show | awk -F': ' '$2!~/^(lo|br0|docker|virbr)/{print $2; exit}')
  ip link add br0 type bridge 2>/dev/null || true
  ip link set "$IF" master br0 2>/dev/null || true
  ip link set br0 up
  ip link set "$IF" up
  ip addr show dev br0 | grep -q "$NODE_IP/" || ip addr add "$NODE_IP/24" dev br0 2>/dev/null || true
fi
mkdir -p /etc/sysctl.d
cat >/etc/sysctl.d/99-vf-routed.conf <<'SYS'
net.ipv4.ip_forward=1
net.ipv4.conf.all.proxy_arp=1
net.ipv4.conf.br0.proxy_arp=1
SYS
sysctl -p /etc/sysctl.d/99-vf-routed.conf >/dev/null 2>&1 || true
ip route replace 212.102.227.0/24 dev br0 scope link 2>/dev/null || true
ip addr show dev br0 | grep -q "${POOL_GW}/" || ip addr add "${POOL_GW}/32" dev br0 2>/dev/null || true
iptables -C FORWARD -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -j ACCEPT 2>/dev/null || true
systemctl is-active libvirtd vf-nginx >/dev/null && echo services=ok || echo services=warn
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2 || true
echo "br0=$(ip -br addr show br0 2>/dev/null | head -1)"
MID

echo "=== 3. NL: SSH key + VF DB + agent auth for HV5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "MID_IP='$MID_IP' MID_PASS='$MID_PASS' bash -s" <<'REMOTE'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
PHP=/opt/virtfusion/php8/bin/php
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
cd /opt/virtfusion/app/control

ssh-keygen -f /root/.ssh/known_hosts -R "$MID_IP" 2>/dev/null || true
ssh-keyscan -H "$MID_IP" >> /root/.ssh/known_hosts 2>/dev/null || true
if [ ! -f /root/.ssh/id_ed25519 ]; then ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519; fi
PUB=$(cat /root/.ssh/id_ed25519.pub)
export SSHPASS="$MID_PASS"
sshpass -e ssh -o StrictHostKeyChecking=no root@"$MID_IP" \
  "mkdir -p /root/.ssh; chmod 700 /root/.ssh; grep -qxF '$PUB' /root/.ssh/authorized_keys 2>/dev/null || echo '$PUB' >> /root/.ssh/authorized_keys; chmod 600 /root/.ssh/authorized_keys"
unset SSHPASS
ssh -o BatchMode=yes -o ConnectTimeout=15 root@"$MID_IP" hostname

NET=$("${MYN[@]}" -e "SELECT id FROM hypervisor_networks WHERE hypervisor_id=5 AND \`primary\`=1 LIMIT 1;")
"${MY[@]}" -e "
  UPDATE hypervisors SET name='DE-midrange', ip='$MID_IP', port=8892, ssh_port=22,
    hypervisor_group_id=5, commissioned=3, enabled=1, maintenance=0, prohibit=0,
    max_cpu=64, max_memory=384408, max_local_hdd=7000 WHERE id=5;
  INSERT IGNORE INTO ip_block_hypervisor (block_id, hypervisor_id) VALUES (6,5);
  INSERT IGNORE INTO ip_block_hypervisor_network (block_id, network_id, hypervisor_id) VALUES (6,$NET,5);
  UPDATE ipv4 SET reserved=1 WHERE block_id=6 AND address=INET_ATON('$MID_IP');
  UPDATE servers SET deleted_at=NOW() WHERE id=693 AND hypervisor_id=5;
  UPDATE ipv4 SET server_id=NULL, interface_id=NULL WHERE server_id=693;"

$PHP <<'PHPEOF'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use Illuminate\Support\Facades\Crypt;
use App\Models\Hypervisor;
$ref = Hypervisor::find(3);
$hv5 = Hypervisor::find(5);
$plain = '';
$chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
$len = strlen(Crypt::decryptString($ref->token));
for ($i = 0; $i < $len; $i++) $plain .= $chars[random_int(0, strlen($chars) - 1)];
$hv5->token = Crypt::encryptString($plain);
$hv5->commissioned = 3;
$hv5->enabled = 1;
$hv5->maintenance = 0;
$hv5->prohibit = 0;
$hv5->ip = '212.102.227.7';
$hv5->save();
$auth = ['ip' => '66.248.206.14', 'token' => substr($plain, 0, 200), 'hash' => bin2hex(random_bytes(32)), 'id' => 5];
file_put_contents('/tmp/hv5-auth.json', json_encode($auth, JSON_UNESCAPED_SLASHES));
echo "token_len=" . strlen((string)$hv5->token) . "\n";
PHPEOF

scp -o BatchMode=yes /tmp/hv5-auth.json root@"$MID_IP":/opt/virtfusion/app/hypervisor/conf/auth.json
ssh -o BatchMode=yes root@"$MID_IP" \
  'chown virtfusion:virtfusion /opt/virtfusion/app/hypervisor/conf/auth.json; chmod 600 /opt/virtfusion/app/hypervisor/conf/auth.json; systemctl restart vf-php8-fpm vf-nginx; supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2'

TOK=$(curl -sk "https://$MID_IP:8892/health" -H "Authorization: Bearer $(python3 -c 'import json;print(json.load(open("/tmp/hv5-auth.json"))["token"])')" -o /dev/null -w '%{http_code}')
echo "agent_health_http=$TOK"
supervisorctl restart vf-queue: vf-queue-hv: vf-queue-control: 2>/dev/null | tail -5 || true
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,LENGTH(token) tok FROM hypervisors WHERE id=5;"
REMOTE

echo "=== 4. Portal DE-mid online ==="
psql "$POSTGRES_DSN" <<'SQL'
UPDATE vps.nodes SET
  external_id = '5',
  vf_name = 'DE-midrange',
  vf_ip = '212.102.227.7',
  status = 'online',
  vf_commissioned = 3,
  vf_enabled = true,
  maintenance_mode = false,
  supported_tiers = ARRAY['midrange']::text[],
  max_memory_mb = 384408,
  max_disk_gb = 7000,
  updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';
SQL

echo "=== 5. Retry stuck order 225 ==="
psql "$POSTGRES_DSN" <<'SQL'
BEGIN;
UPDATE vps.instances
SET state = 'creating',
    external_id = NULL,
    ip_address = NULL,
    worker_poll_claimed_at = NULL,
    worker_poll_claimed_by = NULL,
    updated_at = now()
WHERE id IN (
  SELECT i.id FROM vps.instances i
  JOIN vps.orders o ON o.id = i.order_id
  WHERE o.order_number = 225
) AND state IN ('queued', 'creating', 'error');

UPDATE vps.outbox
SET published = false,
    worker_poll_claimed_at = NULL,
    worker_poll_claimed_by = NULL
WHERE event_type = 'instance.provision_requested'
  AND payload->>'instance_id' IN (
    SELECT i.id::text FROM vps.instances i
    JOIN vps.orders o ON o.id = i.order_id
    WHERE o.order_number = 225
  );
COMMIT;
SELECT o.order_number, i.hostname, i.state, i.external_id, host(i.ip_address) ip
FROM vps.orders o JOIN vps.instances i ON i.order_id = o.id WHERE o.order_number = 225;
SQL

docker restart docker-vps-worker-1
echo "Waiting 120s for provision..."
sleep 120

echo "=== 6. Result ==="
psql "$POSTGRES_DSN" -c "
SELECT o.order_number, i.hostname, i.state, i.external_id, host(i.ip_address) ip, n.name node
FROM vps.orders o JOIN vps.instances i ON i.order_id = o.id
LEFT JOIN vps.nodes n ON n.id = i.node_id WHERE o.order_number = 225;"

docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE '225|midrange|complete provision|provision failed|693|212\.102\.227' | tail -20 || true

SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e \
  "SELECT s.id,s.state,s.hypervisor_id,INET_NTOA(v4.address) ip FROM servers s LEFT JOIN ipv4 v4 ON v4.server_id=s.id WHERE s.hypervisor_id=5 AND s.deleted_at IS NULL ORDER BY s.id DESC LIMIT 5;"
NL

echo "DE_MID_CONNECT_DONE"
