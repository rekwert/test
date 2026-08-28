-- Telegram account linking for Cloud-hustle bot
ALTER TABLE auth.users
  ADD COLUMN IF NOT EXISTS telegram_id BIGINT UNIQUE;

CREATE TABLE IF NOT EXISTS auth.telegram_link_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  telegram_id BIGINT NOT NULL,
  code_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_telegram_link_codes_user ON auth.telegram_link_codes (user_id);
CREATE INDEX IF NOT EXISTS idx_telegram_link_codes_tg ON auth.telegram_link_codes (telegram_id);
