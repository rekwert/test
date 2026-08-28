-- VirtFusion hypervisor sync fields for admin monitoring

ALTER TABLE vps.nodes
    ADD COLUMN IF NOT EXISTS vf_name TEXT,
    ADD COLUMN IF NOT EXISTS vf_ip TEXT,
    ADD COLUMN IF NOT EXISTS vf_hostname TEXT,
    ADD COLUMN IF NOT EXISTS vf_enabled BOOLEAN,
    ADD COLUMN IF NOT EXISTS maintenance_mode BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS vf_commissioned INT,
    ADD COLUMN IF NOT EXISTS max_cpu_cores INT,
    ADD COLUMN IF NOT EXISTS cpu_allocated INT,
    ADD COLUMN IF NOT EXISTS cpu_used_percent REAL,
    ADD COLUMN IF NOT EXISTS max_memory_mb INT,
    ADD COLUMN IF NOT EXISTS memory_allocated_mb INT,
    ADD COLUMN IF NOT EXISTS memory_used_percent REAL,
    ADD COLUMN IF NOT EXISTS max_disk_gb INT,
    ADD COLUMN IF NOT EXISTS disk_allocated_gb INT,
    ADD COLUMN IF NOT EXISTS disk_used_percent REAL,
    ADD COLUMN IF NOT EXISTS vf_server_count INT,
    ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS nodes_external_id_uidx
    ON vps.nodes (external_id)
    WHERE external_id IS NOT NULL AND external_id <> '';
