-- Outbound SMTP (TCP 25/2525) allowlist per VPS. Default closed on HV; admin opens via ticket/tools.

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS smtp_outbound_open BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS instances_smtp_outbound_open_idx
    ON vps.instances (smtp_outbound_open)
    WHERE smtp_outbound_open = true;
