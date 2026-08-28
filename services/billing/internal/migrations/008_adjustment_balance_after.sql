-- Track balance after admin credit/refund for client history UI.
ALTER TABLE billing.adjustments
    ADD COLUMN IF NOT EXISTS balance_after NUMERIC(12, 2);
