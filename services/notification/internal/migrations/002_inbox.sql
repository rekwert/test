-- In-app notification inbox for users (admin broadcasts, system events)

CREATE TABLE IF NOT EXISTS notification.inbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'admin',
    read_at TIMESTAMPTZ,
    sent_by_staff_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS inbox_user_idx
    ON notification.inbox (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS inbox_user_unread_idx
    ON notification.inbox (user_id)
    WHERE read_at IS NULL;
