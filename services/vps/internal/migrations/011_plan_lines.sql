-- PROSTO / Midrange / HUSTLE plan lines (STORAGE not sold yet)

-- Reuse legacy VPS-1..4 UUIDs as PROSTO-1..4 (keeps VIRTFUSION_PLAN_MAP entries valid).
UPDATE vps.plans SET
    name = 'PROSTO-1', cpu = 1, ram_mb = 1024, disk_gb = 20, price_monthly = 365.00, region = 'nl', active = true
WHERE id = '11111111-1111-1111-1111-111111111101';

UPDATE vps.plans SET
    name = 'PROSTO-2', cpu = 2, ram_mb = 2048, disk_gb = 40, price_monthly = 750.00, region = 'nl', active = true
WHERE id = '11111111-1111-1111-1111-111111111102';

UPDATE vps.plans SET
    name = 'PROSTO-3', cpu = 4, ram_mb = 4096, disk_gb = 80, price_monthly = 1500.00, region = 'nl', active = true
WHERE id = '11111111-1111-1111-1111-111111111103';

UPDATE vps.plans SET
    name = 'PROSTO-4', cpu = 6, ram_mb = 8192, disk_gb = 120, price_monthly = 3000.00, region = 'nl', active = true
WHERE id = '11111111-1111-1111-1111-111111111104';

INSERT INTO vps.plans (id, name, cpu, ram_mb, disk_gb, price_monthly, region, active) VALUES
    ('11111111-1111-1111-1111-111111111211', 'Midrange-1', 1, 1024, 20, 475.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111212', 'Midrange-2', 2, 2048, 40, 950.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111213', 'Midrange-3', 4, 4096, 80, 1900.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111214', 'Midrange-4', 6, 8192, 120, 3800.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111221', 'HUSTLE-1', 1, 1024, 20, 680.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111222', 'HUSTLE-2', 2, 2048, 40, 1350.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111223', 'HUSTLE-3', 4, 4096, 80, 2700.00, 'nl', true),
    ('11111111-1111-1111-1111-111111111224', 'HUSTLE-4', 6, 8192, 120, 5400.00, 'nl', true)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    cpu = EXCLUDED.cpu,
    ram_mb = EXCLUDED.ram_mb,
    disk_gb = EXCLUDED.disk_gb,
    price_monthly = EXCLUDED.price_monthly,
    region = EXCLUDED.region,
    active = EXCLUDED.active;
