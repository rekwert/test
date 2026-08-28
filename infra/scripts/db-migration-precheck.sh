#!/usr/bin/env bash
# Pre-migration audit for Machine 3 (PostgreSQL + Redis).
# Run on the current or target DB host, and optionally from Back VPS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
ROLE="${ROLE:-db}" # db | back

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ok()   { echo -e "${GREEN}OK${NC}   $*"; }
warn() { echo -e "${YELLOW}WARN${NC} $*"; }
fail() { echo -e "${RED}FAIL${NC} $*"; }

echo "=== DB migration precheck (role=$ROLE) ==="
echo "host: $(hostname -f 2>/dev/null || hostname)"
echo "time: $(date -Is)"
echo

if [[ "$ROLE" == "db" ]]; then
  echo "--- Storage ---"
  df -h / /var/backups 2>/dev/null || df -h /
  if [[ -d /opt/db/postgres_data ]]; then
    du -sh /opt/db/postgres_data /opt/db/redis_data 2>/dev/null || true
  fi
  echo

  echo "--- Docker ---"
  if command -v docker >/dev/null 2>&1; then
    docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | grep -E 'postgres|redis|pgbouncer|NAMES' || docker ps --format 'table {{.Names}}\t{{.Status}}'
  else
    fail "docker not installed"
  fi
  echo

  echo "--- Network ---"
  ip -4 addr show scope global | awk '/inet / {print "  "$NF": "$2}'
  echo

  echo "--- UFW (5432/6379/6432) ---"
  if command -v ufw >/dev/null 2>&1; then
    ufw status numbered 2>/dev/null | grep -E '5432|6379|6432|OpenSSH' || ufw status | head -20
  else
    warn "ufw not installed"
  fi
  echo

  echo "--- Backups ---"
  BACKUP_DIR="${BACKUP_DIR:-/var/backups/testVPStrade}"
  if [[ -d "$BACKUP_DIR" ]]; then
    ls -lah "$BACKUP_DIR" | tail -5
    latest="$(find "$BACKUP_DIR" -name 'vps_platform_*.sql.gz' -type f -printf '%T@ %p\n' 2>/dev/null | sort -n | tail -1 | cut -d' ' -f2- || true)"
    if [[ -n "$latest" ]]; then
      ok "latest backup: $latest ($(du -h "$latest" | awk '{print $1}'))"
    else
      warn "no vps_platform_*.sql.gz backups in $BACKUP_DIR"
    fi
  else
    warn "backup dir missing: $BACKUP_DIR"
  fi
  echo

  echo "--- Postgres ---"
  PG_CONTAINER="${PG_CONTAINER:-postgres}"
  if docker ps --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
    docker exec "$PG_CONTAINER" psql -U "${POSTGRES_USER:-vps}" -d "${POSTGRES_DB:-vps_platform}" -At -c \
      "SELECT 'db_size='||pg_size_pretty(pg_database_size(current_database()));"
    docker exec "$PG_CONTAINER" psql -U "${POSTGRES_USER:-vps}" -d "${POSTGRES_DB:-vps_platform}" -At -c \
      "SELECT 'migrations='||count(*) FROM platform.schema_migrations;" 2>/dev/null || warn "platform.schema_migrations missing"
    docker exec "$PG_CONTAINER" psql -U "${POSTGRES_USER:-vps}" -d "${POSTGRES_DB:-vps_platform}" -At -c \
      "SELECT 'connections='||count(*) FROM pg_stat_activity WHERE datname=current_database();"
  else
    fail "postgres container '$PG_CONTAINER' not running"
  fi
  echo

  echo "--- Cron ---"
  crontab -l 2>/dev/null | grep -i backup || warn "no backup cron for root"
fi

if [[ "$ROLE" == "back" ]]; then
  if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
  fi
  echo "--- Back VPS DB env ---"
  [[ -n "${DB_VPS_IP:-}" ]] && echo "DB_VPS_IP=$DB_VPS_IP" || warn "DB_VPS_IP unset"
  if [[ -n "${POSTGRES_DSN:-}" ]]; then
    echo "POSTGRES_DSN=$(echo "$POSTGRES_DSN" | sed -E 's/:[^:@]+@/:***@/')"
  else
    fail "POSTGRES_DSN unset"
  fi
  [[ -n "${MIGRATION_DSN:-}" ]] && echo "MIGRATION_DSN=$(echo "$MIGRATION_DSN" | sed -E 's/:[^:@]+@/:***@/')" || warn "MIGRATION_DSN unset (uses POSTGRES_DSN for migrate.sh)"
  [[ -n "${REDIS_URL:-}" ]] && echo "REDIS_URL=$(echo "$REDIS_URL" | sed -E 's/:[^:@]+@/:***@/')"
  echo "POSTGRES_SSL_ALLOW_INSECURE=${POSTGRES_SSL_ALLOW_INSECURE:-false}"
  echo

  echo "--- Connectivity ---"
  if [[ -n "${DB_VPS_IP:-}" ]]; then
    for port in 5432 6432 6379; do
      if timeout 3 bash -c "echo >/dev/tcp/$DB_VPS_IP/$port" 2>/dev/null; then
        ok "tcp $DB_VPS_IP:$port open"
      else
        warn "tcp $DB_VPS_IP:$port closed or filtered"
      fi
    done
  fi
  if command -v psql >/dev/null 2>&1 && [[ -n "${POSTGRES_DSN:-}" ]]; then
    psql "$POSTGRES_DSN" -At -c "SELECT 'pg_ok=1';" && ok "psql POSTGRES_DSN works" || fail "psql POSTGRES_DSN failed"
  fi
fi

echo
echo "=== Precheck done ==="
