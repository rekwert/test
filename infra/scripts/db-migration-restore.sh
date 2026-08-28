#!/usr/bin/env bash
# Restore vps_platform from db-migration-backup.sh dump onto a fresh Postgres.
# Usage: db-migration-restore.sh /path/to/vps_platform_YYYYMMDD_HHMMSS.sql.gz
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 /path/to/vps_platform_*.sql.gz" >&2
  exit 1
fi

DUMP="$1"
ENV_FILE="${ENV_FILE:-/opt/testVPStrade/infra/docker/.env}"
PG_CONTAINER="${PG_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-vps}"
POSTGRES_DB="${POSTGRES_DB:-vps_platform}"

if [[ ! -f "$DUMP" ]]; then
  echo "Dump not found: $DUMP" >&2
  exit 1
fi
gzip -t "$DUMP"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

echo "Waiting for postgres..."
for i in $(seq 1 30); do
  if docker exec "$PG_CONTAINER" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

echo "Restoring $DUMP ..."
export PGCLIENTENCODING=UTF8
zcat "$DUMP" | docker exec -i -e PGCLIENTENCODING=UTF8 "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1

docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c \
  "SELECT 'rows_migrations='||count(*) FROM platform.schema_migrations;"
docker exec "$PG_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c \
  "SELECT 'db_size='||pg_size_pretty(pg_database_size(current_database()));"

echo "Restore done."
