CREATE INDEX IF NOT EXISTS idx_instances_external_id
    ON vps.instances (external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_instances_user_state
    ON vps.instances (user_id, state);

CREATE INDEX IF NOT EXISTS idx_orders_user_id
    ON vps.orders (user_id);

CREATE INDEX IF NOT EXISTS idx_instances_updated_at
    ON vps.instances (updated_at);
