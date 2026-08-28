CREATE SCHEMA IF NOT EXISTS notification;

CREATE TABLE IF NOT EXISTS notification.templates (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    subject TEXT NOT NULL,
    body TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notification.deliveries (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID,
    channel TEXT NOT NULL,
    template TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO notification.templates (name, subject, body) VALUES
    ('welcome', 'Welcome to testVPStrade', 'Your account is ready.'),
    ('instance_ready', 'VPS is ready', 'Your VPS {{hostname}} is now running at {{ip}}.')
ON CONFLICT (name) DO NOTHING;
