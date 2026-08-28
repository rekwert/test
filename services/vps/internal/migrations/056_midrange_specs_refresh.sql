-- Midrange ladder refresh (all regions): disks, RAM, CPU, prices.
UPDATE vps.plans
SET disk_gb = 40
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-1';

UPDATE vps.plans
SET price_monthly = 890.00
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-2';

UPDATE vps.plans
SET ram_mb = 6144,
    disk_gb = 60,
    price_monthly = 1300.00
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-3';

UPDATE vps.plans
SET cpu = 4,
    disk_gb = 100,
    price_monthly = 1499.00
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-4';

UPDATE vps.plans
SET disk_gb = 120
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-5';
