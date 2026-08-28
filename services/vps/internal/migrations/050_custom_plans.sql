-- CUSTOM product line (Claude Code VPS) in all regions; replace STORAGE tier mapping.

UPDATE vps.plans SET tier = 'custom' WHERE lower(name) LIKE 'custom%';

INSERT INTO vps.plans (id, name, cpu, ram_mb, disk_gb, price_monthly, region, active, tier) VALUES
    -- CUSTOM NL
    ('11111111-1111-1111-1111-111111111901', 'CUSTOM-1', 2, 4096, 80, 1199.00, 'nl', true, 'custom'),
    ('11111111-1111-1111-1111-111111111902', 'CUSTOM-2', 4, 8192, 120, 0.00, 'nl', false, 'custom'),
    -- CUSTOM FI
    ('11111111-1111-1111-1111-111111111911', 'CUSTOM-1', 2, 4096, 80, 1199.00, 'fi', true, 'custom'),
    ('11111111-1111-1111-1111-111111111912', 'CUSTOM-2', 4, 8192, 120, 0.00, 'fi', false, 'custom'),
    -- CUSTOM DE
    ('11111111-1111-1111-1111-111111111921', 'CUSTOM-1', 2, 4096, 80, 1199.00, 'de', true, 'custom'),
    ('11111111-1111-1111-1111-111111111922', 'CUSTOM-2', 4, 8192, 120, 0.00, 'de', false, 'custom'),
    -- CUSTOM GB
    ('11111111-1111-1111-1111-111111111931', 'CUSTOM-1', 2, 4096, 80, 1199.00, 'gb', true, 'custom'),
    ('11111111-1111-1111-1111-111111111932', 'CUSTOM-2', 4, 8192, 120, 0.00, 'gb', false, 'custom')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    cpu = EXCLUDED.cpu,
    ram_mb = EXCLUDED.ram_mb,
    disk_gb = EXCLUDED.disk_gb,
    price_monthly = EXCLUDED.price_monthly,
    region = EXCLUDED.region,
    active = EXCLUDED.active,
    tier = EXCLUDED.tier;

-- Enable custom tier on VPS nodes in all sell regions.
UPDATE vps.nodes
SET supported_tiers = (
    SELECT COALESCE(array_agg(DISTINCT tier ORDER BY tier), '{}')
    FROM unnest(supported_tiers || ARRAY['custom']::text[]) AS tier
)
WHERE region IN ('nl', 'fi', 'de', 'gb')
  AND NOT ('custom' = ANY(supported_tiers));
