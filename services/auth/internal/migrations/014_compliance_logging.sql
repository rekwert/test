-- Compliance logging (149-FZ / PP 1526): IP+ports, failed logins, HTTP access, email history.
-- Retention target: 3 years (see infra/scripts/backup-db.sh RETENTION_DAYS).

ALTER TABLE auth.audit_log ADD COLUMN IF NOT EXISTS client_port INT;
ALTER TABLE auth.audit_log ADD COLUMN IF NOT EXISTS server_port INT;

ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS client_port INT;
ALTER TABLE auth.refresh_tokens ADD COLUMN IF NOT EXISTS server_port INT;

CREATE TABLE IF NOT EXISTS auth.login_attempts (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL,
    user_id UUID,
    success BOOLEAN NOT NULL DEFAULT false,
    failure_reason TEXT,
    ip INET,
    client_port INT,
    server_port INT,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_login_attempts_created_at ON auth.login_attempts (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_attempts_email ON auth.login_attempts (email, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_attempts_user_id ON auth.login_attempts (user_id, created_at DESC)
    WHERE user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS auth.http_access_log (
    id BIGSERIAL PRIMARY KEY,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status_code INT NOT NULL,
    client_ip INET,
    client_port INT,
    server_port INT,
    duration_ms INT NOT NULL DEFAULT 0,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_http_access_log_created_at ON auth.http_access_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_http_access_log_path ON auth.http_access_log (path, created_at DESC);

CREATE TABLE IF NOT EXISTS auth.user_email_history (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id),
    email TEXT NOT NULL,
    reason TEXT NOT NULL,
    actor_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_email_history_user ON auth.user_email_history (user_id, created_at DESC);
