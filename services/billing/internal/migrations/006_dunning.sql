-- Dunning: grace period, suspension timestamps, reminders

ALTER TABLE billing.accounts
    ADD COLUMN IF NOT EXISTS past_due_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS grace_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dunning_reminder_at TIMESTAMPTZ;

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS billing_accounts_grace_idx
    ON billing.accounts (grace_until)
    WHERE billing_status = 'past_due' AND grace_until IS NOT NULL;

CREATE INDEX IF NOT EXISTS vps_instances_suspended_idx
    ON vps.instances (suspended_at)
    WHERE billing_status = 'suspended' AND state <> 'deleted';
