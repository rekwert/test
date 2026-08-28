#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -a; source /opt/virtfusion/app/control/.env; set +a
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

echo "=== supervisor ==="
supervisorctl status
echo "=== queued jobs ==="
"${MY[@]}" -e "SELECT id,queue,attempts,available_at,created_at,LEFT(payload,300) payload FROM jobs ORDER BY id DESC LIMIT 10;"
echo "=== failed jobs ==="
"${MY[@]}" -e "SELECT id,connection,queue,failed_at,LEFT(exception,1800) exception,LEFT(payload,500) payload FROM failed_jobs ORDER BY id DESC LIMIT 8;"
echo "=== commission logs ==="
"${MY[@]}" -e "SHOW TABLES;" | python3 -c 'import sys; print("".join(x for x in sys.stdin if "commission" in x.lower() or "hypervisor" in x.lower() or "resource" in x.lower()), end="")'
echo "=== hypervisor jobs/tasks ==="
"${MY[@]}" -e "DESCRIBE hypervisor_jobs; SELECT * FROM hypervisor_jobs ORDER BY id DESC LIMIT 10;"
"${MY[@]}" -e "DESCRIBE hypervisor_tasks; SELECT * FROM hypervisor_tasks ORDER BY id DESC LIMIT 10;"
NL
