-- Payment provider fields for T-Bank acquiring
ALTER TABLE billing.invoices
    ADD COLUMN IF NOT EXISTS provider TEXT,
    ADD COLUMN IF NOT EXISTS provider_payment_id TEXT,
    ADD COLUMN IF NOT EXISTS payment_url TEXT,
    ADD COLUMN IF NOT EXISTS description TEXT,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS billing_invoices_provider_payment_id_idx
    ON billing.invoices (provider_payment_id)
    WHERE provider_payment_id IS NOT NULL;
