CREATE TABLE IF NOT EXISTS billing.adjustments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    amount NUMERIC(12, 2) NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('refund', 'credit', 'debit')),
    reason TEXT,
    staff_id UUID,
    invoice_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS adjustments_user_idx ON billing.adjustments (user_id, created_at DESC);
