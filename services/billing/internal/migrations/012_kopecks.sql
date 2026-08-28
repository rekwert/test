-- Money as integer kopecks (1 RUB = 100). Keeps legacy NUMERIC columns in sync during transition.

ALTER TABLE billing.accounts
    ADD COLUMN IF NOT EXISTS balance_kopecks BIGINT NOT NULL DEFAULT 0;

ALTER TABLE billing.invoices
    ADD COLUMN IF NOT EXISTS amount_kopecks BIGINT,
    ADD COLUMN IF NOT EXISTS bonus_amount_kopecks BIGINT NOT NULL DEFAULT 0;

ALTER TABLE billing.adjustments
    ADD COLUMN IF NOT EXISTS amount_kopecks BIGINT;

-- Backfill from existing NUMERIC columns.
UPDATE billing.accounts
SET balance_kopecks = ROUND(balance * 100)::bigint
WHERE balance_kopecks = 0 AND balance <> 0;

UPDATE billing.accounts
SET balance_kopecks = 0
WHERE balance_kopecks = 0 AND balance = 0;

UPDATE billing.invoices
SET amount_kopecks = ROUND(amount * 100)::bigint
WHERE amount_kopecks IS NULL;

UPDATE billing.invoices
SET bonus_amount_kopecks = ROUND(COALESCE(bonus_amount, 0) * 100)::bigint
WHERE bonus_amount_kopecks = 0 AND COALESCE(bonus_amount, 0) <> 0;

UPDATE billing.adjustments
SET amount_kopecks = ROUND(amount * 100)::bigint
WHERE amount_kopecks IS NULL;

ALTER TABLE billing.invoices
    ALTER COLUMN amount_kopecks SET NOT NULL;

ALTER TABLE billing.adjustments
    ALTER COLUMN amount_kopecks SET NOT NULL;

-- Sync NUMERIC ↔ kopecks on write (dual-write period).
CREATE OR REPLACE FUNCTION billing.sync_balance_kopecks()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.balance_kopecks IS NOT NULL AND NEW.balance_kopecks <> 0 THEN
            NEW.balance := (NEW.balance_kopecks::numeric / 100);
        ELSIF NEW.balance IS NOT NULL THEN
            NEW.balance_kopecks := ROUND(NEW.balance * 100)::bigint;
        END IF;
    ELSE
        IF NEW.balance_kopecks IS DISTINCT FROM OLD.balance_kopecks THEN
            NEW.balance := (NEW.balance_kopecks::numeric / 100);
        ELSIF NEW.balance IS DISTINCT FROM OLD.balance THEN
            NEW.balance_kopecks := ROUND(NEW.balance * 100)::bigint;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_accounts_balance_kopecks ON billing.accounts;
CREATE TRIGGER trg_accounts_balance_kopecks
    BEFORE INSERT OR UPDATE ON billing.accounts
    FOR EACH ROW EXECUTE FUNCTION billing.sync_balance_kopecks();

CREATE OR REPLACE FUNCTION billing.sync_invoice_kopecks()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.amount_kopecks IS NOT NULL THEN
            NEW.amount := (NEW.amount_kopecks::numeric / 100);
        ELSIF NEW.amount IS NOT NULL THEN
            NEW.amount_kopecks := ROUND(NEW.amount * 100)::bigint;
        END IF;
        IF NEW.bonus_amount_kopecks IS NOT NULL AND NEW.bonus_amount_kopecks <> 0 THEN
            NEW.bonus_amount := (NEW.bonus_amount_kopecks::numeric / 100);
        ELSIF NEW.bonus_amount IS NOT NULL THEN
            NEW.bonus_amount_kopecks := ROUND(COALESCE(NEW.bonus_amount, 0) * 100)::bigint;
        END IF;
    ELSE
        IF NEW.amount_kopecks IS DISTINCT FROM OLD.amount_kopecks THEN
            NEW.amount := (NEW.amount_kopecks::numeric / 100);
        ELSIF NEW.amount IS DISTINCT FROM OLD.amount THEN
            NEW.amount_kopecks := ROUND(NEW.amount * 100)::bigint;
        END IF;
        IF NEW.bonus_amount_kopecks IS DISTINCT FROM OLD.bonus_amount_kopecks THEN
            NEW.bonus_amount := (NEW.bonus_amount_kopecks::numeric / 100);
        ELSIF NEW.bonus_amount IS DISTINCT FROM OLD.bonus_amount THEN
            NEW.bonus_amount_kopecks := ROUND(COALESCE(NEW.bonus_amount, 0) * 100)::bigint;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_invoices_kopecks ON billing.invoices;
CREATE TRIGGER trg_invoices_kopecks
    BEFORE INSERT OR UPDATE ON billing.invoices
    FOR EACH ROW EXECUTE FUNCTION billing.sync_invoice_kopecks();

CREATE OR REPLACE FUNCTION billing.sync_adjustment_kopecks()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.amount_kopecks IS NOT NULL THEN
            NEW.amount := (NEW.amount_kopecks::numeric / 100);
        ELSIF NEW.amount IS NOT NULL THEN
            NEW.amount_kopecks := ROUND(NEW.amount * 100)::bigint;
        END IF;
    ELSE
        IF NEW.amount_kopecks IS DISTINCT FROM OLD.amount_kopecks THEN
            NEW.amount := (NEW.amount_kopecks::numeric / 100);
        ELSIF NEW.amount IS DISTINCT FROM OLD.amount THEN
            NEW.amount_kopecks := ROUND(NEW.amount * 100)::bigint;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_adjustments_kopecks ON billing.adjustments;
CREATE TRIGGER trg_adjustments_kopecks
    BEFORE INSERT OR UPDATE ON billing.adjustments
    FOR EACH ROW EXECUTE FUNCTION billing.sync_adjustment_kopecks();
