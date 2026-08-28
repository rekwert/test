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

if [[ -z "${POSTGRES_DSN:-}" ]]; then
  echo "POSTGRES_DSN is not set (use infra/docker/.env on Back machine)" >&2
  exit 1
fi

# DDL through PgBouncer transaction pool is unsafe — prefer direct Postgres when set.
MIGRATE_DSN="${MIGRATION_DSN:-$POSTGRES_DSN}"
if [[ -n "${MIGRATION_DSN:-}" ]]; then
  echo "Using MIGRATION_DSN for DDL (direct Postgres recommended)"
fi

run_psql() {
  psql "$MIGRATE_DSN" "$@"
}

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found. Install postgresql-client: apt install -y postgresql-client" >&2
  exit 1
fi

ensure_schema() {
  run_psql -v ON_ERROR_STOP=1 <<'SQL'
CREATE SCHEMA IF NOT EXISTS platform;
CREATE TABLE IF NOT EXISTS platform.schema_migrations (
  service text NOT NULL,
  filename text NOT NULL,
  applied_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (service, filename)
);
SQL
}

migration_applied() {
  local service="$1"
  local filename="$2"
  run_psql -tAc \
    "SELECT EXISTS(SELECT 1 FROM platform.schema_migrations WHERE service = '$service' AND filename = '$filename')"
}

record_migration() {
  local service="$1"
  local filename="$2"
  run_psql -v ON_ERROR_STOP=1 -c \
    "INSERT INTO platform.schema_migrations (service, filename) VALUES ('$service', '$filename') ON CONFLICT DO NOTHING"
}

run_sql() {
  local file="$1"
  echo "==> $(basename "$file")"
  run_psql -v ON_ERROR_STOP=1 -f "$file"
}

run_dir_versioned() {
  local service="$1"
  local dir="$2"
  if [[ ! -d "$dir" ]]; then
    echo "skip missing dir: $dir" >&2
    return 0
  fi
  local f
  while IFS= read -r -d '' f; do
    local filename
    filename="$(basename "$f")"
    if [[ "$(migration_applied "$service" "$filename")" == "t" ]]; then
      echo "skip $service/$filename (already applied)"
      continue
    fi
    run_sql "$f"
    record_migration "$service" "$filename"
  done < <(find "$dir" -maxdepth 1 -name '*.sql' -print0 | sort -z)
}

run_single_versioned() {
  local service="$1"
  local file="$2"
  local filename
  filename="$(basename "$file")"
  if [[ "$(migration_applied "$service" "$filename")" == "t" ]]; then
    echo "skip $service/$filename (already applied)"
    return 0
  fi
  run_sql "$file"
  record_migration "$service" "$filename"
}

echo "Running migrations against remote DB (platform.schema_migrations)..."

ensure_schema

# Same services/files as platformmigrate.Apply in Go services.
run_dir_versioned "auth" "$ROOT/services/auth/internal/migrations"
run_dir_versioned "billing" "$ROOT/services/billing/internal/migrations"
run_dir_versioned "vps" "$ROOT/services/vps/internal/migrations"
run_dir_versioned "notification" "$ROOT/services/notification/internal/migrations"
run_dir_versioned "support" "$ROOT/services/support/internal/migrations"

echo "Migrations done."
