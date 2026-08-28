-- Self-service: metrics, snapshots, backups

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS metrics_updated_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS vps.instance_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES vps.instances(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'creating',
    external_id TEXT,
    size_gb INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vps.instance_backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES vps.instances(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    external_id TEXT,
    schedule TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS instance_snapshots_instance_idx
    ON vps.instance_snapshots (instance_id, created_at DESC);

CREATE INDEX IF NOT EXISTS instance_backups_instance_idx
    ON vps.instance_backups (instance_id, created_at DESC);
