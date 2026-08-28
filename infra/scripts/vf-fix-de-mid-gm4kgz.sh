#!/bin/bash
set -euo pipefail
ROOT=/opt/testVPStrade
source "$ROOT/infra/docker/.env.probe"
source "$ROOT/infra/docker/.env"
DE_MID_PASS="${DE_MID_SSH_PASS:?}"
INSTANCE_ID="8d8a46f7-c421-4fb8-b540-c0932299df95"
VF_SERVER_ID="669"

echo "=== 1. NL: SS groups + packages for HV group 5 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

EXISTS=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_group_hv_group WHERE hypervisor_group_id=5;")
if [[ "$EXISTS" == "0" ]]; then
  NEXT=$("${MYN[@]}" -e "SELECT COALESCE(MAX(group_id),0)+1 FROM ss_group_hv_group;")
  "${MY[@]}" -e "INSERT INTO ss_group_hv_group (group_id, hypervisor_group_id, name, label, \`order\`) VALUES ($NEXT, 5, 'DE Mid', 'DE Mid', 5);"
fi
GID=$("${MYN[@]}" -e "SELECT group_id FROM ss_group_hv_group WHERE hypervisor_group_id=5 LIMIT 1;")
EXISTS2=$("${MYN[@]}" -e "SELECT COUNT(*) FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;")
if [[ "$EXISTS2" == "0" ]]; then
  "${MY[@]}" -e "INSERT INTO ss_grp_hv_grp_pkg_grp (package_group_id, hypervisor_group_id, group_id, \`order\`) VALUES (1, 5, $GID, 5);"
fi

for PKG in 10 17 18 19 20 21; do
  "${MY[@]}" -e "INSERT IGNORE INTO ss_package_group_package (package_group_id, package_id, \`order\`) VALUES (1, $PKG, $PKG);" 2>/dev/null || true
done

"${MY[@]}" -e "UPDATE hypervisors SET commissioned=3, enabled=1, maintenance=0, prohibit=0 WHERE id=5;"
"${MY[@]}" -e "SELECT * FROM ss_group_hv_group WHERE hypervisor_group_id=5;"
"${MY[@]}" -e "SELECT * FROM ss_grp_hv_grp_pkg_grp WHERE hypervisor_group_id=5;"
"${MY[@]}" -e "SELECT spgp.package_id, sp.name FROM ss_package_group_package spgp JOIN server_packages sp ON sp.id=spgp.package_id WHERE spgp.package_id IN (17,18) ORDER BY spgp.package_id;"

supervisorctl restart vf-queue: vf-queue-hv: vf-queue-control: 2>/dev/null | tail -5 || true
NL

echo ""
echo "=== 2. DE-mid: libvirtd on boot + queue ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 \
  "ssh -o BatchMode=yes root@66.151.40.165 bash -s" <<'MID'
set -euo pipefail
systemctl enable libvirtd
systemctl start libvirtd
grep -q '212.102.227.0/24' /etc/rc.local 2>/dev/null || {
  echo 'ip route add 212.102.227.0/24 dev br0 2>/dev/null || true' >> /etc/rc.local
  chmod +x /etc/rc.local
}
sysctl -w net.ipv4.conf.all.proxy_arp=1 net.ipv4.conf.br0.proxy_arp=1 >/dev/null
supervisorctl restart vf-queue-hv: 2>/dev/null | tail -2
echo "libvirtd=$(systemctl is-active libvirtd) kvm=$(test -e /dev/kvm && echo ok)"
MID

echo ""
echo "=== 3. Retire broken VF shell ${VF_SERVER_ID}, fresh allocate ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<REMOTE
set -a; source /opt/virtfusion/app/control/.env; set +a
mysql -h127.0.0.1 -u"\$DB_USERNAME" -p"\$DB_PASSWORD" "\$DB_DATABASE" \
  -e "UPDATE servers SET deleted_at=NOW(), state='deleted' WHERE id=${VF_SERVER_ID} AND deleted_at IS NULL;"
REMOTE

psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<SQL
UPDATE vps.instances
SET external_id = NULL,
    ip_address = NULL,
    state = 'creating',
    updated_at = now(),
    provider_meta = provider_meta - 'provision_error'
WHERE id = '${INSTANCE_ID}';

UPDATE vps.nodes
SET vf_commissioned = 3,
    status = 'online',
    vf_enabled = true,
    maintenance_mode = false,
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005';
SQL

echo ""
echo "=== 4. Restart worker ==="
cd "$ROOT/infra/docker"
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1

echo ""
echo "=== 5. Wait 120s ==="
sleep 120

echo ""
echo "=== 6. Status ==="
psql "$POSTGRES_DSN" -c \
  "SELECT id, hostname, state, external_id, ip_address::text FROM vps.instances WHERE id='${INSTANCE_ID}';"

docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE '8d8a46f7|gm4kgz|retry allocate|retry build|complete provision|not commissioned|669|670|671|672|673|674|675' | tail -40 || true

source "$ROOT/infra/docker/.env"
EXT=$(psql "$POSTGRES_DSN" -t -A -c "SELECT COALESCE(external_id,'') FROM vps.instances WHERE id='${INSTANCE_ID}';")
if [[ -n "$EXT" ]]; then
  curl -sk "${VIRTFUSION_API_URL}/servers/${EXT}" -H "Authorization: Bearer ${VIRTFUSION_API_KEY}" \
    | python3 -c 'import sys,json;s=json.load(sys.stdin)["data"];print("vf",s["id"],"state",s["state"],"commission",s["commissionStatus"],"hv",s["hypervisorId"])'
fi

echo "FIX_GM4KGZ_DONE"
