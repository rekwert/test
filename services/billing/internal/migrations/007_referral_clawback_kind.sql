-- Allow referral clawback adjustment kind.
DO $$
DECLARE
  cname text;
BEGIN
  SELECT con.conname INTO cname
  FROM pg_constraint con
  JOIN pg_class rel ON rel.oid = con.conrelid
  JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
  WHERE nsp.nspname = 'billing'
    AND rel.relname = 'adjustments'
    AND con.contype = 'c'
    AND pg_get_constraintdef(con.oid) ILIKE '%kind%'
    AND pg_get_constraintdef(con.oid) ILIKE '%refund%'
  LIMIT 1;
  IF cname IS NOT NULL THEN
    EXECUTE format('ALTER TABLE billing.adjustments DROP CONSTRAINT %I', cname);
  END IF;

  ALTER TABLE billing.adjustments
    ADD CONSTRAINT adjustments_kind_check
    CHECK (kind IN ('refund', 'credit', 'debit', 'referral_clawback'));
END $$;
