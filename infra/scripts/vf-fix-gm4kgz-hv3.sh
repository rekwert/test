#!/bin/bash
set -eo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
source /opt/testVPStrade/infra/docker/.env

echo "=== NL: ensure midrange pkg 18 on HV group 3 ==="
SSHPASS="$NL_SSH_PASS" sshpass -e ssh root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
for PKG in 10 17 18 19 20 21; do
  "${MY[@]}" -e "INSERT IGNORE INTO ss_package_group_package (package_group_id, package_id, \`order\`) VALUES (1, $PKG, $PKG);" 2>/dev/null || true
done
"${MY[@]}" -e "UPDATE servers SET deleted_at=NOW() WHERE id IN (679,680,681) AND deleted_at IS NULL;"
"${MY[@]}" -e "SELECT id,hypervisor_id,state FROM servers WHERE id IN (679,680,681);"
supervisorctl restart vf-queue: 2>/dev/null | tail -2
NL

echo "=== reset gm4kgz ==="
psql "$POSTGRES_DSN" -c "UPDATE vps.instances SET external_id=NULL, ip_address=NULL, state='creating', updated_at=now() WHERE hostname='vps-gm4kgz';"
docker restart docker-vps-worker-1
sleep 120
bash /tmp/check-gm4kgz.sh 2>/dev/null || true
docker logs docker-vps-worker-1 --since 3m 2>&1 | grep -iE 'gm4kgz|8d8a46f7|retry|complete provision|680|681|682|683' | tail -25
