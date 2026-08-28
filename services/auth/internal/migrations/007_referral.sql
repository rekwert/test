CREATE SCHEMA IF NOT EXISTS referral;

CREATE TABLE IF NOT EXISTS referral.codes (
    user_id UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    code TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS referral.registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    referrer_user_id UUID NOT NULL REFERENCES auth.users(id),
    referred_user_id UUID NOT NULL UNIQUE REFERENCES auth.users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'registered'
        CHECK (status IN ('registered', 'paid', 'earning')),
    friend_bonus_paid BOOLEAN NOT NULL DEFAULT false,
    total_earned NUMERIC(12, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_referral_regs_referrer
    ON referral.registrations(referrer_user_id);

CREATE TABLE IF NOT EXISTS referral.earnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    referrer_user_id UUID NOT NULL REFERENCES auth.users(id),
    registration_id UUID NOT NULL REFERENCES referral.registrations(id) ON DELETE CASCADE,
    amount NUMERIC(12, 2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_referral_earnings_referrer
    ON referral.earnings(referrer_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS referral.link_clicks (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_referral_clicks_code
    ON referral.link_clicks(code);
