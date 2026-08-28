ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS ip_address INET;
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS user_agent TEXT;
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS browser TEXT NOT NULL DEFAULT 'Unknown';
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS os TEXT NOT NULL DEFAULT 'Unknown';
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS device_type TEXT NOT NULL DEFAULT 'desktop';
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS auth_method TEXT NOT NULL DEFAULT 'password';
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_active ON auth.refresh_tokens(user_id, last_active_at DESC)
  WHERE revoked_at IS NULL;
