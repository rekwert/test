-- Release IPs from deleted rows before adding active IP uniqueness guard.
UPDATE vps.instances SET ip_address = NULL WHERE state = 'deleted' AND ip_address IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_instances_ip_active
    ON vps.instances (ip_address)
    WHERE state NOT IN ('deleted') AND ip_address IS NOT NULL;
