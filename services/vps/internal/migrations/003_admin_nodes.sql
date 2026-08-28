-- Admin: hypervisor nodes, audit trail, instance node binding

CREATE TABLE IF NOT EXISTS vps.nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    region TEXT NOT NULL DEFAULT 'moscow',
    external_id TEXT,
    status TEXT NOT NULL DEFAULT 'online',
    capacity_instances INT NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE vps.instances ADD COLUMN IF NOT EXISTS node_id UUID REFERENCES vps.nodes(id);

CREATE TABLE IF NOT EXISTS vps.admin_actions (
    id BIGSERIAL PRIMARY KEY,
    staff_id UUID,
    user_id UUID,
    instance_id UUID,
    action TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS admin_actions_user_idx ON vps.admin_actions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS admin_actions_instance_idx ON vps.admin_actions (instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS instances_node_idx ON vps.instances (node_id);

INSERT INTO vps.nodes (id, name, region, status, capacity_instances)
VALUES ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01', 'Node-1', 'moscow', 'online', 100)
ON CONFLICT (id) DO NOTHING;
