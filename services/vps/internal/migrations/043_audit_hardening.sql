-- Billing due scan: include grace_period instances (matches ListDueInstances query).
DROP INDEX IF EXISTS vps.instances_billing_due_idx;
CREATE INDEX IF NOT EXISTS instances_billing_due_idx
    ON vps.instances (next_billing_at)
    WHERE billing_status IN ('active', 'grace_period')
      AND state <> 'deleted';

-- Abuse worker scan rotation.
CREATE INDEX IF NOT EXISTS idx_instances_abuse_scan
    ON vps.instances (metrics_updated_at NULLS FIRST, updated_at ASC)
    WHERE state = 'running'
      AND ip_address IS NOT NULL
      AND COALESCE(product_type, 'vps') = 'vps'
      AND COALESCE(provider, 'virtfusion') NOT IN ('hetzner_robot', 'hostkey');

-- One active password-change job per instance.
CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_active_password_once
    ON vps.outbox (event_type, (payload->>'instance_id'))
    WHERE published = false
      AND event_type = 'instance.password_change_requested';
