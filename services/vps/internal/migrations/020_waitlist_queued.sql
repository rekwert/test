-- Waitlist: instances may wait in state=queued until a node has free capacity.
-- SSH public keys selected at order time are kept for later provision.

ALTER TABLE vps.instances
  ADD COLUMN IF NOT EXISTS provision_ssh_keys JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS instances_queued_created_idx
  ON vps.instances (created_at ASC)
  WHERE state = 'queued';
