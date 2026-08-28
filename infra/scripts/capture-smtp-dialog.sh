#!/usr/bin/env bash
# Capture SMTP dialogue for Selectel support (run on back server).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
RECIP="${1:-korosteliov.danil@mail.ru}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE" >&2
  exit 1
fi

command -v swaks >/dev/null || {
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq swaks libnet-ssleay-perl
}

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

LOG="/tmp/smtp-dialog-$(date -u +%Y%m%d-%H%M%S).log"
REDACTED="/tmp/smtp-dialog-redacted-$(date -u +%Y%m%d-%H%M%S).log"

swaks \
  --to "$RECIP" \
  --from "$SMTP_FROM" \
  --server "$SMTP_HOST:$SMTP_PORT" \
  --tls-on-connect \
  --auth LOGIN \
  --auth-user "$SMTP_USER" \
  --auth-password "$SMTP_PASS" \
  --h-Subject "SMTP-diag-cloud-hustle" \
  --h-Reply-To "support@cloud-hustle.com" \
  --body "SMTP-diagnostic-test-for-Selectel-ticket" \
  2>&1 | tee "$LOG"

# Redact AUTH password line for sharing with support.
sed -E \
  -e 's/^ ~> [A-Za-z0-9+\/=]{12,}$/ ~> [REDACTED_AUTH]/' \
  -e 's/^<~  334 UGFzc3dvcmQ6/<~  334 [PASSWORD_PROMPT]/' \
  "$LOG" > "$REDACTED"

echo "Full log (contains secret): $LOG"
echo "Redacted log (attach to ticket): $REDACTED"
echo "Recipient: $RECIP"
