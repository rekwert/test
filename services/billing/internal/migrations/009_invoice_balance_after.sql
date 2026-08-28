-- Snapshot account balance after paid topups/charges for history UI.

ALTER TABLE billing.invoices
    ADD COLUMN IF NOT EXISTS balance_after NUMERIC(12, 2);
