-- Async email delivery queue for notification worker

ALTER TABLE notification.deliveries
    ADD COLUMN IF NOT EXISTS to_email TEXT,
    ADD COLUMN IF NOT EXISTS subject TEXT,
    ADD COLUMN IF NOT EXISTS body TEXT,
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error TEXT,
    ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS deliveries_pending_email_idx
    ON notification.deliveries (created_at ASC)
    WHERE status = 'pending' AND channel = 'email';
