-- Free week: one claim per user (atomic with order transaction).
CREATE TABLE IF NOT EXISTS vps.free_week_claims (
    user_id UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE RESTRICT
);

INSERT INTO vps.free_week_claims (user_id)
SELECT DISTINCT i.user_id
FROM vps.instances i
WHERE COALESCE((i.provider_meta->>'free_week')::boolean, false)
   OR COALESCE((i.provider_meta->>'trial')::boolean, false)
ON CONFLICT (user_id) DO NOTHING;

-- VPS worker poll claim (prevents duplicate AllocateServer under multiple workers).
ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS worker_poll_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS worker_poll_claimed_by TEXT;

CREATE INDEX IF NOT EXISTS idx_instances_creating_poll
    ON vps.instances (created_at ASC)
    WHERE state IN ('creating', 'running')
      AND ip_address IS NULL
      AND COALESCE(provider, 'virtfusion') NOT IN ('hetzner_robot', 'hostkey');

-- Outbox performance + idempotency for active provision/reinstall events.
CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON vps.outbox (id ASC)
    WHERE published = false;

CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_active_provision_once
    ON vps.outbox (event_type, (payload->>'instance_id'))
    WHERE published = false
      AND event_type IN ('instance.provision_requested', 'instance.reinstall_requested');

-- Email delivery worker claim (status processing).
CREATE INDEX IF NOT EXISTS idx_deliveries_processing_reclaim
    ON notification.deliveries (updated_at ASC)
    WHERE status = 'processing' AND channel = 'email';
