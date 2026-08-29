#!/usr/bin/env bash
# Resume Sunbeam after prepare-node (juju bootstrap + cluster bootstrap + portal env).
set -euo pipefail
export PATH="/snap/bin:${PATH}"
REPO="${OPENSTACK_WORKDIR:-/opt/openstack-portal}"
SUNBEAM_USER="${SUNBEAM_USER:-sunbeam}"
LOG="/root/sunbeam-resume.log"
exec > >(tee -a "$LOG") 2>&1

echo "=== $(date -Is) resume Sunbeam ==="

if command -v docker >/dev/null 2>&1; then
  docker stop docker-gateway-1 2>/dev/null || true
fi

KEY="/root/.ssh/id_ed25519_sunbeam_local"
if [[ ! -f "$KEY" ]]; then
  echo "Missing $KEY — run restart-sunbeam-install.sh first"
  exit 1
fi

run_as_sunbeam() {
  ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "${SUNBEAM_USER}@127.0.0.1" "export PATH=/snap/bin:\$PATH; $*"
}

if ! run_as_sunbeam "juju show-controller" 2>/dev/null | grep -q controller; then
  echo "=== juju bootstrap localhost ==="
  run_as_sunbeam "juju bootstrap localhost"
fi

echo "=== sunbeam cluster bootstrap ==="
if [[ ! -f /home/$SUNBEAM_USER/demo-openrc ]]; then
  run_as_sunbeam "sunbeam cluster bootstrap --accept-defaults"
fi

echo "=== sunbeam configure ==="
if [[ ! -f /home/$SUNBEAM_USER/demo-openrc ]]; then
  run_as_sunbeam "sunbeam configure --accept-defaults --openrc ~/demo-openrc"
fi

install -m 600 "/home/$SUNBEAM_USER/demo-openrc" /root/demo-openrc
# shellcheck disable=SC1091
source /root/demo-openrc
bash "$REPO/infra/openstack/bootstrap-dev.sh" | tee /root/openstack-bootstrap.log

echo "=== verify ==="
openstack token issue
openstack network list
cat /root/openstack-portal-dev.env
echo "=== DONE $(date -Is) ==="
