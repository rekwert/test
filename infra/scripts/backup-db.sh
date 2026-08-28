#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

BACKUP_DIR="${BACKUP_DIR:-/var/backups/testVPStrade}"
RETENTION_DAYS="${RETENTION_DAYS:-1095}"
STAMP="$(date +%Y%m%d_%H%M%S)"

if [[ -z "${POSTGRES_DSN:-}" ]]; then
  echo "POSTGRES_DSN is not set" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"
OUT="${1:-$BACKUP_DIR/vps_platform_${STAMP}.sql.gz}"

echo "Backup -> $OUT"
pg_dump "$POSTGRES_DSN" | gzip > "$OUT"

find "$BACKUP_DIR" -name 'vps_platform_*.sql.gz' -mtime +"$RETENTION_DAYS" -delete
echo "Backup done."
