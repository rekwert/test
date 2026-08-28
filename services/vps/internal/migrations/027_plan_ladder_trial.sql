-- Expand PROSTO / Midrange / HUSTLE ladders and add TRIAL line.
-- Keep legacy UUIDs; add new SKUs for larger steps.

INSERT INTO vps.plans (id, name, cpu, ram_mb, disk_gb, price_monthly, region, active, tier) VALUES
    -- TRIAL
    ('11111111-1111-1111-1111-111111111601', 'TRIAL-1', 1, 2048, 10, 149.00, 'nl', true, 'trial'),
    ('11111111-1111-1111-1111-111111111602', 'TRIAL-1', 1, 2048, 10, 149.00, 'fi', true, 'trial'),

    -- PROSTO NL
    ('11111111-1111-1111-1111-111111111101', 'PROSTO-1', 1, 1024, 10, 299.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111102', 'PROSTO-2', 1, 2048, 20, 449.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111103', 'PROSTO-3', 2, 4096, 40, 749.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111104', 'PROSTO-4', 4, 4096, 40, 949.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111105', 'PROSTO-5', 6, 8192, 60, 1499.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111106', 'PROSTO-6', 6, 12288, 80, 2199.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111107', 'PROSTO-7', 8, 24576, 80, 3499.00, 'nl', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111108', 'PROSTO-8', 8, 24576, 120, 4199.00, 'nl', true, 'prosto'),

    -- PROSTO FI
    ('11111111-1111-1111-1111-111111111501', 'PROSTO-1', 1, 1024, 10, 299.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111502', 'PROSTO-2', 1, 2048, 20, 449.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111503', 'PROSTO-3', 2, 4096, 40, 749.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111504', 'PROSTO-4', 4, 4096, 40, 949.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111505', 'PROSTO-5', 6, 8192, 60, 1499.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111506', 'PROSTO-6', 6, 12288, 80, 2199.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111507', 'PROSTO-7', 8, 24576, 80, 3499.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111508', 'PROSTO-8', 8, 24576, 120, 4199.00, 'fi', true, 'prosto'),

    -- Midrange NL
    ('11111111-1111-1111-1111-111111111211', 'Midrange-1', 1, 2048, 20, 475.00, 'nl', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111212', 'Midrange-2', 2, 4096, 40, 950.00, 'nl', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111213', 'Midrange-3', 4, 4096, 40, 1200.00, 'nl', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111214', 'Midrange-4', 6, 8192, 60, 1900.00, 'nl', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111215', 'Midrange-5', 6, 12288, 80, 2800.00, 'nl', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111216', 'Midrange-6', 8, 24576, 80, 4500.00, 'nl', true, 'midrange'),
    ('11111111-1111-1111-1111-111111111217', 'Midrange-7', 8, 24576, 120, 5400.00, 'nl', true, 'midrange'),

    -- HUSTLE NL
    ('11111111-1111-1111-1111-111111111221', 'HUSTLE-1', 1, 2048, 20, 680.00, 'nl', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111222', 'HUSTLE-2', 2, 4096, 40, 1350.00, 'nl', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111223', 'HUSTLE-3', 4, 4096, 40, 1700.00, 'nl', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111224', 'HUSTLE-4', 6, 8192, 60, 2700.00, 'nl', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111225', 'HUSTLE-5', 6, 12288, 80, 4000.00, 'nl', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111226', 'HUSTLE-6', 8, 24576, 80, 6500.00, 'nl', true, 'hustle'),
    ('11111111-1111-1111-1111-111111111227', 'HUSTLE-7', 8, 24576, 120, 7800.00, 'nl', true, 'hustle')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    cpu = EXCLUDED.cpu,
    ram_mb = EXCLUDED.ram_mb,
    disk_gb = EXCLUDED.disk_gb,
    price_monthly = EXCLUDED.price_monthly,
    region = EXCLUDED.region,
    active = EXCLUDED.active,
    tier = EXCLUDED.tier;

-- Allow trial on nodes that already sell prosto (NL / FI).
UPDATE vps.nodes
SET supported_tiers = (
    SELECT ARRAY(
        SELECT DISTINCT t
        FROM unnest(COALESCE(supported_tiers, ARRAY[]::text[]) || ARRAY['trial']) AS t
        ORDER BY 1
    )
)
WHERE region IN ('nl', 'fi')
  AND status = 'online'
  AND 'prosto' = ANY (COALESCE(supported_tiers, ARRAY[]::text[]));
