-- Admin manual VM block (separate from billing suspend and abuse_hold).

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS admin_block BOOLEAN NOT NULL DEFAULT false;

-- Legacy invalid state from early admin tools.
UPDATE vps.instances
SET state = 'stopped',
    admin_block = true,
    updated_at = now()
WHERE state = 'blocked';

CREATE INDEX IF NOT EXISTS instances_admin_block_idx
    ON vps.instances (admin_block)
    WHERE admin_block = true;
