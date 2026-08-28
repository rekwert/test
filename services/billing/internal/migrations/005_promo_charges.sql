-- Promo codes, charge invoices, recurring billing support

CREATE TABLE IF NOT EXISTS billing.promo_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN (
        'credit',
        'topup_bonus_percent',
        'topup_bonus_fixed',
        'charge_discount_percent'
    )),
    value NUMERIC(12, 2) NOT NULL,
    min_amount NUMERIC(12, 2) NOT NULL DEFAULT 0,
    max_redemptions INT,
    redemption_count INT NOT NULL DEFAULT 0,
    per_user_limit INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    active BOOLEAN NOT NULL DEFAULT true,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS promo_codes_code_lower_idx ON billing.promo_codes (lower(code));

CREATE TABLE IF NOT EXISTS billing.promo_redemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    promo_id UUID NOT NULL REFERENCES billing.promo_codes(id),
    user_id UUID NOT NULL,
    invoice_id UUID REFERENCES billing.invoices(id),
    bonus_amount NUMERIC(12, 2) NOT NULL DEFAULT 0,
    discount_amount NUMERIC(12, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS promo_redemptions_user_idx
    ON billing.promo_redemptions (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS promo_redemptions_promo_idx
    ON billing.promo_redemptions (promo_id);

CREATE TABLE IF NOT EXISTS billing.promo_entitlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    promo_id UUID NOT NULL REFERENCES billing.promo_codes(id),
    discount_percent NUMERIC(5, 2) NOT NULL,
    instance_id UUID,
    expires_at TIMESTAMPTZ,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS promo_entitlements_user_idx
    ON billing.promo_entitlements (user_id)
    WHERE active = true;

ALTER TABLE billing.invoices
    ADD COLUMN IF NOT EXISTS invoice_type TEXT NOT NULL DEFAULT 'topup',
    ADD COLUMN IF NOT EXISTS instance_id UUID,
    ADD COLUMN IF NOT EXISTS promo_id UUID REFERENCES billing.promo_codes(id),
    ADD COLUMN IF NOT EXISTS bonus_amount NUMERIC(12, 2) NOT NULL DEFAULT 0;

ALTER TABLE billing.invoices DROP CONSTRAINT IF EXISTS invoices_invoice_type_check;
ALTER TABLE billing.invoices ADD CONSTRAINT invoices_invoice_type_check
    CHECK (invoice_type IN ('topup', 'charge'));

CREATE INDEX IF NOT EXISTS invoices_type_user_idx
    ON billing.invoices (user_id, invoice_type, created_at DESC);

CREATE INDEX IF NOT EXISTS invoices_instance_idx
    ON billing.invoices (instance_id, created_at DESC)
    WHERE instance_id IS NOT NULL;

INSERT INTO billing.promo_codes (code, kind, value, min_amount, description)
SELECT 'WELCOME10', 'topup_bonus_percent', 10, 100, '10% bonus on balance top-up'
WHERE NOT EXISTS (SELECT 1 FROM billing.promo_codes WHERE lower(code) = 'welcome10');
