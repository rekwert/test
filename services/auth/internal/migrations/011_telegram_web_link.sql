-- One-time tokens for linking Telegram from the website settings toggle.
CREATE TABLE IF NOT EXISTS auth.telegram_web_link_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_telegram_web_link_tokens_user
  ON auth.telegram_web_link_tokens (user_id);
