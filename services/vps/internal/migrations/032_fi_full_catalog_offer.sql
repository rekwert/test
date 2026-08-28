-- Landing/LK show the full FI offer (prosto + midrange + hustle).
-- Sale still gated by node supported_tiers (FI stays trial-only → available=false).

UPDATE vps.plans
SET active = true
WHERE region = 'fi'
  AND tier IN ('prosto', 'midrange', 'hustle');

INSERT INTO vps.plans (id, name, cpu, ram_mb, disk_gb, price_monthly, region, active, tier) VALUES
    -- Midrange FI
    ('11111111-1111-1111-1111-111111111511', 'Midrange-1', 1, 2048, 20, 475.00, 'fi', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111512', 'Midrange-2', 2, 4096, 40, 950.00, 'fi', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111513', 'Midrange-3', 4, 4096, 40, 1200.00, 'fi', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111514', 'Midrange-4', 6, 8192, 60, 1900.00, 'fi', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111515', 'Midrange-5', 6, 12288, 80, 2800.00, 'fi', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111516', 'Midrange-6', 8, 24576, 80, 4500.00, 'fi', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111517', 'Midrange-7', 8, 24576, 120, 5400.00, 'fi', true, 'midrange'),

    -- HUSTLE FI
    ('11111111-1111-1111-1111-111111111521', 'HUSTLE-1', 1, 2048, 20, 680.00, 'fi', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111522', 'HUSTLE-2', 2, 4096, 40, 1350.00, 'fi', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111523', 'HUSTLE-3', 4, 4096, 40, 1700.00, 'fi', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111524', 'HUSTLE-4', 6, 8192, 60, 2700.00, 'fi', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111525', 'HUSTLE-5', 6, 12288, 80, 4000.00, 'fi', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111526', 'HUSTLE-6', 8, 24576, 80, 6500.00, 'fi', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111527', 'HUSTLE-7', 8, 24576, 120, 7800.00, 'fi', true, 'hustle')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    cpu = EXCLUDED.cpu,
    ram_mb = EXCLUDED.ram_mb,
    disk_gb = EXCLUDED.disk_gb,
    price_monthly = EXCLUDED.price_monthly,
    region = EXCLUDED.region,
    active = EXCLUDED.active,
    tier = EXCLUDED.tier;

-- Keep FI node trial-only so ListPlans marks paid FI plans available=false.
UPDATE vps.nodes
SET supported_tiers = ARRAY['trial']::text[]
WHERE region = 'fi'
  AND (name = 'FI-1' OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002');
