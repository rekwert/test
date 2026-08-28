#!/usr/bin/env bash
# Install daily VirtFusion zombie purge on the panel host (66.248.206.14).
# Zombies = servers.state=allocated AND commissioned=0 (VF shell rows blocking max_servers).
set -euo pipefail

SCRIPT="${1:-/opt/testVPStrade/infra/scripts/vf-purge-zombie-servers.sh}"
ENV_PURGE="${ENV_PURGE:-/opt/testVPStrade/infra/docker/.env.purge}"
ENV_DOCKER="${ENV_DOCKER:-/opt/testVPStrade/infra/docker/.env}"
# Source portal DSN so purge skips vps.instances.external_id (active provision).
CRON_LINE="17 4 * * * root bash -c 'set -a; [ -f $ENV_PURGE ] && . $ENV_PURGE; [ -f $ENV_DOCKER ] && . $ENV_DOCKER; set +a; bash $SCRIPT --apply' >> /var/log/vf-zombie-purge.log 2>&1"

if [[ ! -f "$SCRIPT" ]]; then
  echo "Missing $SCRIPT" >&2
  exit 1
fi
chmod +x "$SCRIPT"

if ! command -v psql >/dev/null 2>&1 && [ ! -x /usr/bin/psql ]; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y postgresql-client
fi

CRON_FILE=/etc/cron.d/vf-zombie-purge
cat >"$CRON_FILE" <<EOF
# VirtFusion orphan shell servers (allocated, not commissioned)
$CRON_LINE
EOF
chmod 644 "$CRON_FILE"
echo "Installed $CRON_FILE"
cat "$CRON_FILE"
