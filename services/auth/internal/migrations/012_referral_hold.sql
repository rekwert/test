-- Referral hold: delay balance credit by 30 days; support clawback on refund.
ALTER TABLE referral.earnings
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'credited',
    ADD COLUMN IF NOT EXISTS available_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS source_user_id UUID,
    ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credited_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS clawed_at TIMESTAMPTZ;

UPDATE referral.earnings
SET status = 'credited',
    available_at = COALESCE(available_at, created_at),
    credited_at = COALESCE(credited_at, created_at)
WHERE status = 'credited' OR status IS NULL OR status = '';

DO $$
BEGIN
  ALTER TABLE referral.earnings DROP CONSTRAINT IF EXISTS referral_earnings_status_check;
  ALTER TABLE referral.earnings
    ADD CONSTRAINT referral_earnings_status_check
    CHECK (status IN ('pending', 'credited', 'clawed_back', 'cancelled'));
END $$;

CREATE INDEX IF NOT EXISTS idx_referral_earnings_pending_due
    ON referral.earnings (available_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_referral_earnings_source_user
    ON referral.earnings (source_user_id)
    WHERE source_user_id IS NOT NULL;
