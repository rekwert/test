-- Instance billing cycle for monthly charges

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS next_billing_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS billing_period_days INT NOT NULL DEFAULT 30;

UPDATE vps.instances
SET next_billing_at = created_at + (billing_period_days || ' days')::interval
WHERE next_billing_at IS NULL;

CREATE INDEX IF NOT EXISTS instances_billing_due_idx
    ON vps.instances (next_billing_at)
    WHERE billing_status = 'active' AND state <> 'deleted';
