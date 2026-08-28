CREATE TABLE IF NOT EXISTS vps.ip_assignment_log (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    instance_id UUID REFERENCES vps.instances (id) ON DELETE SET NULL,
    ip_address  INET NOT NULL,
    event       TEXT NOT NULL CHECK (event IN ('assigned', 'released')),
    source      TEXT NOT NULL,
    old_ip      INET,
    actor_id    UUID REFERENCES auth.users (id) ON DELETE SET NULL,
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ip_assignment_log_user_created
    ON vps.ip_assignment_log (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ip_assignment_log_instance_created
    ON vps.ip_assignment_log (instance_id, created_at DESC)
    WHERE instance_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ip_assignment_log_ip
    ON vps.ip_assignment_log (ip_address);
