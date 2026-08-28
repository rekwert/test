-- Fix kopecks sync triggers: on INSERT with only amount/balance (no kopecks), derive kopecks instead of nulling amount.

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
