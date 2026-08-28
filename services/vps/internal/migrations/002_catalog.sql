-- Catalog reference tables + order columns (seed for prod DB sync)

DROP INDEX IF EXISTS vps.plans_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS plans_name_region_key ON vps.plans (name, region);

CREATE TABLE IF NOT EXISTS vps.os_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    family TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS vps.software_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    active BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS vps.os_software (
    os_id TEXT NOT NULL REFERENCES vps.os_templates(id),
    software_id TEXT NOT NULL REFERENCES vps.software_profiles(id),
    PRIMARY KEY (os_id, software_id)
);

ALTER TABLE vps.orders ADD COLUMN IF NOT EXISTS os_template_id TEXT;
ALTER TABLE vps.orders ADD COLUMN IF NOT EXISTS software_profile_id TEXT;
ALTER TABLE vps.orders ADD COLUMN IF NOT EXISTS hostname TEXT;

INSERT INTO vps.software_profiles (id, name, description, labels) VALUES
    ('clean', 'Clean OS', 'Minimal OS install', '{"ru":"Чистая ОС","en":"Clean OS"}'),
    ('3x-ui', '3X-UI', '3X-UI panel (Xray)', '{"ru":"3X-UI (Xray)","en":"3X-UI (Xray)"}'),
    ('python3', 'Python 3', 'Python 3 + pip + venv', '{"ru":"Python 3","en":"Python 3"}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO vps.plans (id, name, cpu, ram_mb, disk_gb, price_monthly, region) VALUES
    ('11111111-1111-1111-1111-111111111101', 'VPS-1', 1, 1024, 20, 149.00, 'moscow'),
    ('11111111-1111-1111-1111-111111111102', 'VPS-2', 2, 2048, 40, 299.00, 'moscow'),
    ('11111111-1111-1111-1111-111111111103', 'VPS-3', 4, 4096, 80, 599.00, 'moscow'),
    ('11111111-1111-1111-1111-111111111104', 'VPS-4', 6, 8192, 120, 999.00, 'moscow')
ON CONFLICT (id) DO NOTHING;
