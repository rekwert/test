#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env

echo "=== VirtFusion control: healthy flags and actual FI capacity ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -euo pipefail
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
"${MY[@]}" -e "UPDATE hypervisors SET enabled=1,maintenance=0,prohibit=0,max_cpu=12,max_memory=62117,max_local_hdd=800 WHERE id=2;"
"${MY[@]}" -e "UPDATE hypervisor_storage SET capacity=800,enabled=1 WHERE hypervisor_id=2;"
"${MY[@]}" -e "UPDATE hypervisors SET enabled=1,maintenance=0,prohibit=0 WHERE id=3;"
"${MY[@]}" -e "SELECT id,name,ip,commissioned,enabled,maintenance,prohibit,max_cpu,max_memory,max_local_hdd,LENGTH(token) token_len FROM hypervisors WHERE id IN (2,3);"
"${MY[@]}" -e "SELECT hypervisor_id,type,bridge,\`primary\`,\`default\`,enabled FROM hypervisor_networks WHERE hypervisor_id IN (2,3);"
"${MY[@]}" -e "SELECT hypervisor_id,name,path,capacity,enabled FROM hypervisor_storage WHERE hypervisor_id IN (2,3);"
NL

echo "=== Portal: restore FI node availability ==="
psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 <<'SQL'
UPDATE vps.nodes SET
  name='FI-1',
  external_id='2',
  status='online',
  maintenance_mode=false,
  vf_enabled=true,
  vf_commissioned=3,
  capacity_instances=50,
  supported_tiers=ARRAY['prosto']::text[],
  updated_at=now()
WHERE id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002';
SELECT name,region,status,external_id,maintenance_mode,vf_enabled,vf_commissioned,capacity_instances,supported_tiers
FROM vps.nodes WHERE id IN (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003'
) ORDER BY region;
SQL

echo "=== Physical hosts ==="
for spec in "FI 95.216.1.155 $FI_SSH_PASS" "DE-prosto 185.84.224.84 $DE_SSH_PASS"; do
  read -r label ip pass <<<"$spec"
  echo "--- $label ---"
  SSHPASS="$pass" sshpass -e ssh -n -o StrictHostKeyChecking=no root@"$ip" \
    'printf "hostname="; hostname; systemctl is-active libvirtd vf-nginx supervisor; printf "port8892="; ss -tln | awk "/:8892 /{n++} END{print n+0}"; printf "defined_vms="; virsh list --all --name | awk "NF{n++} END{print n+0}"; printf "running_vms="; virsh list --name | awk "NF{n++} END{print n+0}"'
done

echo "=== Restart VPS worker and verify regional catalog ==="
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml restart vps-worker 2>/dev/null || docker restart docker-vps-worker-1
sleep 8
curl -fsS http://127.0.0.1:8080/api/v1/catalog/regions || true
echo
