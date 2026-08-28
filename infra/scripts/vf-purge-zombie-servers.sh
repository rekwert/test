#!/bin/bash
# Purge uncommissioned allocated VF server rows blocking new provisioning.
# Skips VF server ids linked from portal vps.instances.external_id (active provision).
set -euo pipefail
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
DBH="$DB_HOST"
DBU="$DB_USERNAME"
DBP="$DB_PASSWORD"
DBN="$DB_DATABASE"
my() { mysql -h"$DBH" -u"$DBU" -p"$DBP" "$DBN" -N -e "$1"; }
myx() { mysql -h"$DBH" -u"$DBU" -p"$DBP" "$DBN" -e "$1"; }

load_postgres_dsn() {
  if [ -n "${POSTGRES_DSN:-}" ]; then
    return 0
  fi
  local f val
  for f in \
    /opt/testVPStrade/infra/docker/.env.purge \
    /opt/testVPStrade/infra/docker/.env; do
    if [ -f "$f" ]; then
      val=$(grep -m1 '^POSTGRES_DSN=' "$f" | cut -d= -f2- | tr -d '\r"' || true)
      if [ -n "$val" ]; then
        POSTGRES_DSN="$val"
        export POSTGRES_DSN
        return 0
      fi
    fi
  done
  return 1
}

load_portal_linked_vf_ids() {
  LINKED_IDS=""
  PORTAL_LINKED_COUNT=0
  if ! load_postgres_dsn; then
    echo "portal_linked_ids=skipped (POSTGRES_DSN not set)"
    return 0
  fi
  PSQL_BIN="${PSQL_BIN:-}"
  if [ -z "$PSQL_BIN" ]; then
    PSQL_BIN="$(command -v psql 2>/dev/null || true)"
  fi
  if [ -z "$PSQL_BIN" ] && [ -x /usr/bin/psql ]; then
    PSQL_BIN=/usr/bin/psql
  fi
  if [ -z "$PSQL_BIN" ]; then
    echo "portal_linked_ids=skipped (psql not found)" >&2
    return 0
  fi

  local raw
  raw=$("$PSQL_BIN" "$POSTGRES_DSN" -At -v ON_ERROR_STOP=1 -c "
    SELECT DISTINCT external_id
    FROM vps.instances
    WHERE external_id IS NOT NULL
      AND TRIM(external_id) <> ''
      AND external_id ~ '^[0-9]+$'
  " 2>/dev/null) || {
    echo "portal_linked_ids=error (psql failed)" >&2
    return 1
  }

  local id
  while IFS= read -r id; do
    [ -z "$id" ] && continue
    if [ -z "$LINKED_IDS" ]; then
      LINKED_IDS="$id"
    else
      LINKED_IDS="$LINKED_IDS,$id"
    fi
    PORTAL_LINKED_COUNT=$((PORTAL_LINKED_COUNT + 1))
  done <<< "$raw"

  if [ "$PORTAL_LINKED_COUNT" -gt 0 ]; then
    echo "portal_linked_ids=$PORTAL_LINKED_COUNT ($LINKED_IDS)"
  else
    echo "portal_linked_ids=0"
  fi
}

APPLY="${1:-}"
load_portal_linked_vf_ids

ZOMBIE_WHERE="commissioned=0 AND state='allocated'"
if [ -n "$LINKED_IDS" ]; then
  ZOMBIE_WHERE="${ZOMBIE_WHERE} AND id NOT IN (${LINKED_IDS})"
fi

COUNT=$(my "SELECT COUNT(*) FROM servers WHERE ${ZOMBIE_WHERE};")
PROTECTED=0
if [ -n "$LINKED_IDS" ]; then
  PROTECTED=$(my "SELECT COUNT(*) FROM servers WHERE commissioned=0 AND state='allocated' AND id IN (${LINKED_IDS});" || echo 0)
fi
echo "zombie_servers=$COUNT protected_by_portal=$PROTECTED apply=${APPLY:-dry-run}"

if [ "$COUNT" = "0" ]; then
  echo "nothing to purge"
  exit 0
fi

TABLES=$(my "SELECT DISTINCT TABLE_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='${DBN}' AND COLUMN_NAME='server_id';")
echo "tables_with_server_id:"
echo "$TABLES"

if [ "$APPLY" != "--apply" ]; then
  echo "dry-run only; pass --apply to delete"
  exit 0
fi

IDS=$(my "SELECT GROUP_CONCAT(id) FROM servers WHERE ${ZOMBIE_WHERE};")
echo "purging ids: $IDS"

for t in $TABLES; do
  if [ "$t" = "servers" ]; then
    continue
  fi
  n=$(my "SELECT COUNT(*) FROM ${t} WHERE server_id IN (SELECT id FROM servers WHERE ${ZOMBIE_WHERE});" 2>/dev/null || echo 0)
  if [ "${n:-0}" != "0" ]; then
    echo "DELETE FROM $t ($n rows)"
    myx "DELETE FROM ${t} WHERE server_id IN (SELECT id FROM servers WHERE ${ZOMBIE_WHERE});" || true
  fi
done

echo "DELETE FROM servers"
myx "DELETE FROM servers WHERE ${ZOMBIE_WHERE};"

STORAGE=$(my "SELECT COUNT(*) FROM hypervisor_storage WHERE hypervisor_id=1;")
if [ "$STORAGE" = "0" ]; then
  echo "INSERT hypervisor_storage default mountpoint"
  myx "INSERT INTO hypervisor_storage (hypervisor_id,name,path,type,capacity,storage_type,enabled,\`default\`,storage_data,created_at,updated_at) VALUES (1,'Local (Default mountpoint)','/home/vf-data/disk','mountpoint',1000,0,1,1,'[]',NOW(),NOW());"
fi

echo "remaining_servers=$(my 'SELECT COUNT(*) FROM servers;')"
echo "hypervisor_storage=$(my 'SELECT COUNT(*) FROM hypervisor_storage;')"
