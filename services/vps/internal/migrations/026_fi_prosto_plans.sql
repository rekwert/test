-- Finland PROSTO rows (separate UUIDs per region; same VirtFusion packages 1–4).

INSERT INTO vps.plans (id, name, cpu, ram_mb, disk_gb, price_monthly, region, active, tier) VALUES
    ('11111111-1111-1111-1111-111111111501', 'PROSTO-1', 1, 1024, 20, 365.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111502', 'PROSTO-2', 2, 2048, 40, 750.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111503', 'PROSTO-3', 4, 4096, 80, 1500.00, 'fi', true, 'prosto'),
    ('11111111-1111-1111-1111-111111111504', 'PROSTO-4', 6, 8192, 120, 3000.00, 'fi', true, 'prosto')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    cpu = EXCLUDED.cpu,
    ram_mb = EXCLUDED.ram_mb,
    disk_gb = EXCLUDED.disk_gb,
    price_monthly = EXCLUDED.price_monthly,
    region = EXCLUDED.region,
    active = EXCLUDED.active,
    tier = EXCLUDED.tier;
