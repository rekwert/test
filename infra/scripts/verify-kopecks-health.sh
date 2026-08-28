#!/usr/bin/env bash
# Post-kopecks migration integrity + trigger smoke tests (read-only checks + rolled-back writes).
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
  echo "POSTGRES_DSN is not set" >&2
  exit 1
fi

PSQL=(psql "$POSTGRES_DSN" -v ON_ERROR_STOP=1 -X)

echo "=== Kopecks migration health check ==="
echo

run_check() {
  local name="$1"
  local sql="$2"
  if "${PSQL[@]}" -tAc "$sql" | grep -qx 't\|1\|ok\|OK'; then
    echo "OK   $name"
    return 0
  fi
  local out
  out=$("${PSQL[@]}" -tAc "$sql" 2>&1) || true
  echo "FAIL $name"
  echo "     $out"
  return 1
}

FAIL=0
check() {
  run_check "$1" "$2" || FAIL=$((FAIL + 1))
}

# --- Data integrity (existing rows) ---
check "accounts balance sync" \
  "SELECT CASE WHEN COUNT(*) = 0 THEN 'ok' ELSE 'fail' END FROM billing.accounts WHERE balance IS DISTINCT FROM (balance_kopecks::numeric / 100)"

check "invoices amount sync" \
  "SELECT CASE WHEN COUNT(*) = 0 THEN 'ok' ELSE 'fail' END FROM billing.invoices WHERE amount IS DISTINCT FROM (amount_kopecks::numeric / 100)"

check "invoices bonus sync" \
  "SELECT CASE WHEN COUNT(*) = 0 THEN 'ok' ELSE 'fail' END FROM billing.invoices WHERE bonus_amount IS DISTINCT FROM (bonus_amount_kopecks::numeric / 100)"

check "adjustments amount sync" \
  "SELECT CASE WHEN COUNT(*) = 0 THEN 'ok' ELSE 'fail' END FROM billing.adjustments WHERE amount IS DISTINCT FROM (amount_kopecks::numeric / 100)"

check "no null invoice amounts" \
  "SELECT CASE WHEN COUNT(*) = 0 THEN 'ok' ELSE 'fail' END FROM billing.invoices WHERE amount IS NULL OR amount_kopecks IS NULL"

check "migration 015 applied" \
  "SELECT CASE WHEN EXISTS(SELECT 1 FROM platform.schema_migrations WHERE service='billing' AND filename='015_balance_kopecks_constraint.sql') THEN 'ok' ELSE 'fail' END"

check "migration 013 applied" \
  "SELECT CASE WHEN EXISTS(SELECT 1 FROM platform.schema_migrations WHERE service='billing' AND filename='013_fix_kopecks_trigger_insert.sql') THEN 'ok' ELSE 'fail' END"

# --- Trigger smoke tests (rolled back) ---
"${PSQL[@]}" <<'SQL'
DO $$
DECLARE
  uid uuid;
  inv_id uuid;
  bal numeric;
  bk bigint;
  amt numeric;
  ak bigint;
BEGIN
  SELECT user_id INTO uid FROM billing.accounts LIMIT 1;
  IF uid IS NULL THEN
    RAISE EXCEPTION 'no billing account for smoke test';
  END IF;

  -- charge invoice (VPS order path)
  INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, balance_after)
  VALUES (uid, 215, 'paid', 'smoke charge', 'balance', 'charge', 1000)
  RETURNING id, amount, amount_kopecks INTO inv_id, amt, ak;
  IF amt IS NULL OR ak IS NULL OR ak <> 21500 THEN
    RAISE EXCEPTION 'charge invoice trigger failed: % / %', amt, ak;
  END IF;
  RAISE NOTICE 'OK smoke charge invoice';

  -- topup invoice (amount + kopecks explicit)
  INSERT INTO billing.invoices (user_id, amount, amount_kopecks, status, description, provider, invoice_type, bonus_amount, bonus_amount_kopecks)
  VALUES (uid, 100, 10000, 'pending', 'smoke topup', 'tbank', 'topup', 0, 0);
  RAISE NOTICE 'OK smoke topup invoice';

  -- robokassa-style (amount only)
  INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, promo_id, bonus_amount)
  VALUES (uid, 50, 'pending', 'smoke robokassa', 'robokassa', 'topup', NULL, 0);
  RAISE NOTICE 'OK smoke robokassa invoice';

  -- adjustment
  INSERT INTO billing.adjustments (user_id, amount, kind, reason)
  VALUES (uid, 10, 'credit', 'smoke adjustment');
  RAISE NOTICE 'OK smoke adjustment';

  -- balance update (charge worker path)
  UPDATE billing.accounts SET balance = balance - 1 WHERE user_id = uid
  RETURNING balance, balance_kopecks INTO bal, bk;
  IF bal IS NULL OR bk IS NULL THEN
    RAISE EXCEPTION 'balance update sync failed';
  END IF;
  UPDATE billing.accounts SET balance = balance + 1 WHERE user_id = uid;
  RAISE NOTICE 'OK smoke balance update';

  RAISE EXCEPTION 'rollback smoke tests' USING ERRCODE = 'P0001';
EXCEPTION
  WHEN SQLSTATE 'P0001' THEN
    NULL; -- expected rollback
END $$;
SQL
echo "OK   trigger smoke tests (rolled back)"

# --- Partitioning insert smoke ---
"${PSQL[@]}" <<'SQL'
DO $$
BEGIN
  INSERT INTO auth.audit_log (actor_id, action, entity, entity_id, metadata, ip)
  VALUES (NULL, 'smoke', 'health', 'test', '{}'::jsonb, NULL);
  INSERT INTO notification.deliveries (user_id, channel, template, status)
  VALUES (NULL, 'email', 'welcome', 'pending');
  INSERT INTO vps.outbox (event_type, payload)
  VALUES ('smoke.health', '{}'::jsonb);
  RAISE EXCEPTION 'rollback partition smoke' USING ERRCODE = 'P0001';
EXCEPTION
  WHEN SQLSTATE 'P0001' THEN NULL;
END $$;
SQL
echo "OK   partitioning insert smoke (rolled back)"

# --- Non-billing writes ---
"${PSQL[@]}" <<'SQL'
DO $$
DECLARE tid uuid;
BEGIN
  INSERT INTO support.tickets (user_id, client_email, subject, priority)
  VALUES (NULL, 'smoke@test.local', 'health smoke', 'normal')
  RETURNING id INTO tid;
  INSERT INTO support.ticket_messages (ticket_id, author_email, is_staff, body)
  VALUES (tid, 'smoke@test.local', false, 'smoke');
  RAISE EXCEPTION 'rollback support smoke' USING ERRCODE = 'P0001';
EXCEPTION
  WHEN SQLSTATE 'P0001' THEN NULL;
END $$;
SQL
echo "OK   support ticket smoke (rolled back)"

echo
if [[ "$FAIL" -gt 0 ]]; then
  echo "=== RESULT: $FAIL integrity check(s) FAILED ==="
  exit 1
fi
echo "=== RESULT: all checks passed ==="
