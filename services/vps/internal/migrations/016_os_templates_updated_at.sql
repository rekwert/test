ALTER TABLE vps.os_templates
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT now();
