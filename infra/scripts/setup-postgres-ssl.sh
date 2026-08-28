#!/usr/bin/env bash
# Enable self-signed TLS on Postgres (run on DB host). After this, set POSTGRES_DSN sslmode=require.
set -euo pipefail

ENV_FILE="${ENV_FILE:-/opt/testVPStrade/infra/docker/.env}"
SSL_DIR="${SSL_DIR:-/opt/postgres-ssl}"

if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
fi

mkdir -p "$SSL_DIR"
if [[ ! -f "$SSL_DIR/server.key" ]]; then
  openssl req -new -x509 -days 825 -nodes -text \
    -out "$SSL_DIR/server.crt" \
    -keyout "$SSL_DIR/server.key" \
    -subj "/CN=postgres-vps-platform"
  chmod 600 "$SSL_DIR/server.key"
  chown -R 999:999 "$SSL_DIR" 2>/dev/null || true
fi

echo "Certificates in $SSL_DIR"
echo "Add to postgres command (docker-compose.db.yml):"
echo "  - -c ssl=on"
echo "  - -c ssl_cert_file=/var/lib/postgresql/ssl/server.crt"
echo "  - -c ssl_key_file=/var/lib/postgresql/ssl/server.key"
echo "Mount $SSL_DIR -> /var/lib/postgresql/ssl in compose, then sslmode=require in POSTGRES_DSN"
