#!/usr/bin/env bash
# Repair UTF-8 text corrupted during DB migration by syncing from a good pg_dump backup.
# Usage: db-repair-utf8-from-backup.sh /path/to/vps_platform_*.sql.gz
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 /path/to/vps_platform_*.sql.gz" >&2
  exit 1
fi

DUMP="$1"
PG_CONTAINER="${PG_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-vps}"
MAIN_DB="${POSTGRES_DB:-vps_platform}"
REPAIR_DB="${REPAIR_DB:-vps_repair_utf8}"

if [[ ! -f "$DUMP" ]]; then
  echo "Dump not found: $DUMP" >&2
  exit 1
fi
gzip -t "$DUMP"

echo "Creating repair database ${REPAIR_DB}..."
docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${REPAIR_DB}' AND pid <> pg_backend_pid();" >/dev/null 2>&1 || true
docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 -c \
  "DROP DATABASE IF EXISTS ${REPAIR_DB};"
docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 -c \
  "CREATE DATABASE ${REPAIR_DB} ENCODING 'UTF8';"

echo "Restoring backup into ${REPAIR_DB} (UTF-8)..."
export PGCLIENTENCODING=UTF8
zcat "$DUMP" | docker exec -i -e PGCLIENTENCODING=UTF8 "$PG_CONTAINER" \
  psql -U "$POSTGRES_USER" -d "$REPAIR_DB" -v ON_ERROR_STOP=1 -q

repair_sql() {
  docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d "$MAIN_DB" -v ON_ERROR_STOP=1 -c "$1"
}

echo "Syncing text columns from ${REPAIR_DB} -> ${MAIN_DB}..."

repair_sql "CREATE EXTENSION IF NOT EXISTS dblink;"

repair_sql "
UPDATE auth.ssh_keys t
SET name = s.name
FROM dblink('dbname=${REPAIR_DB}','SELECT id::text, name FROM auth.ssh_keys') AS s(id text, name text)
WHERE t.id::text = s.id AND t.name IS DISTINCT FROM s.name;
"

repair_sql "
UPDATE auth.users t
SET display_name = s.display_name
FROM dblink('dbname=${REPAIR_DB}','SELECT id::text, display_name FROM auth.users WHERE display_name IS NOT NULL') AS s(id text, display_name text)
WHERE t.id::text = s.id AND t.display_name IS DISTINCT FROM s.display_name;
"

repair_sql "
UPDATE support.tickets t
SET subject = s.subject
FROM dblink('dbname=${REPAIR_DB}','SELECT id::text, subject FROM support.tickets') AS s(id text, subject text)
WHERE t.id::text = s.id AND t.subject IS DISTINCT FROM s.subject;
"

repair_sql "
UPDATE support.ticket_messages t
SET body = s.body
FROM dblink('dbname=${REPAIR_DB}','SELECT id::text, body FROM support.ticket_messages') AS s(id text, body text)
WHERE t.id::text = s.id AND t.body IS DISTINCT FROM s.body;
"

repair_sql "
UPDATE vps.instances t
SET hostname = s.hostname
FROM dblink('dbname=${REPAIR_DB}','SELECT id::text, hostname FROM vps.instances WHERE hostname IS NOT NULL') AS s(id text, hostname text)
WHERE t.id::text = s.id AND t.hostname IS DISTINCT FROM s.hostname;
"

repair_sql "
UPDATE billing.invoices t
SET description = s.description
FROM dblink('dbname=${REPAIR_DB}','SELECT id::text, description FROM billing.invoices WHERE description IS NOT NULL') AS s(id text, description text)
WHERE t.id::text = s.id AND t.description IS DISTINCT FROM s.description;
"

echo "Sample check (ssh key names with question marks should be 0):"
docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d "$MAIN_DB" -At -c \
  "SELECT count(*) FROM auth.ssh_keys WHERE name ~ '^[?]+$';"

echo "Repair done."
