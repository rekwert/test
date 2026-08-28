ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS pending_password_change TEXT;
