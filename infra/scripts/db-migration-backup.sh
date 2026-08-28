#!/usr/bin/env bash
# Full logical backup before DB host migration.
# Works with:
#   - POSTGRES_DSN in env / .env
#   - docker container named postgres (current prod at /opt/db)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/testVPStrade}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
STAMP="$(date +%Y%m%d_%H%M%S)"
PG_CONTAINER="${PG_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-vps}"
POSTGRES_DB="${POSTGRES_DB:-vps_platform}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

mkdir -p "$BACKUP_DIR"
OUT="${1:-$BACKUP_DIR/vps_platform_${STAMP}.sql.gz}"
TMP="${OUT%.gz}"

echo "Backup -> $OUT"

if [[ -n "${POSTGRES_DSN:-}" ]] && command -v pg_dump >/dev/null 2>&1; then
  pg_dump "$POSTGRES_DSN" | gzip > "$OUT"
elif docker ps --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
  docker exec "$PG_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl | gzip > "$OUT"
else
  echo "No POSTGRES_DSN+pg_dump and no docker container '$PG_CONTAINER'" >&2
  exit 1
fi

# Verify archive is readable gzip with SQL header
if ! gzip -t "$OUT"; then
  echo "Backup verification failed: corrupt gzip" >&2
  exit 1
fi
if ! zcat "$OUT" | head -5 | grep -q 'PostgreSQL database dump'; then
  echo "Backup verification failed: unexpected dump content" >&2
  exit 1
fi

find "$BACKUP_DIR" -name 'vps_platform_*.sql.gz' -mtime +"$RETENTION_DAYS" -delete
ls -lah "$OUT"
echo "Backup done."
