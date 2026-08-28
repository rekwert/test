#!/usr/bin/env bash
# OpenStack control on bare metal. Sunbeam on 24.04, MicroStack on 22.04.
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0"
  exit 1
fi

CODENAME="$(. /etc/os-release && echo "${VERSION_CODENAME:-unknown}")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "$CODENAME" in
  noble)
    echo "Ubuntu 24.04 — Sunbeam install..."
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq snapd curl
    systemctl enable --now snapd.socket
    sleep 5
    snap install openstack --channel=2024.1/stable || snap install openstack
    sunbeam prepare-node-script --bootstrap | bash -x
    export PATH="/snap/bin:$PATH"
    sunbeam cluster bootstrap --accept-defaults
    sunbeam configure --accept-defaults --openrc /root/demo-openrc
    # shellcheck disable=SC1091
    source /root/demo-openrc
    bash "$SCRIPT_DIR/bootstrap-dev.sh" | tee /root/openstack-bootstrap.log
    ;;
  jammy)
    echo "Ubuntu 22.04 — Sunbeam needs 24.04; using MicroStack snap..."
    bash "$SCRIPT_DIR/control-install-microstack.sh"
    ;;
  *)
    echo "Unsupported Ubuntu codename: $CODENAME (need jammy 22.04 or noble 24.04)"
    exit 1
    ;;
esac

echo ""
echo "Control install done."
echo "  cat /root/openstack-portal-dev.env"
