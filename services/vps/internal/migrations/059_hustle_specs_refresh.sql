-- HUSTLE ladder refresh (all regions): disks, CPU/RAM, prices.
UPDATE vps.plans SET disk_gb = 100 WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-1';

UPDATE vps.plans
SET disk_gb = 120, price_monthly = 710.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-2';

UPDATE vps.plans
SET cpu = 2, ram_mb = 6144, disk_gb = 150, price_monthly = 1999.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-3';

UPDATE vps.plans
SET cpu = 4, disk_gb = 180, price_monthly = 2650.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-4';

UPDATE vps.plans
SET disk_gb = 200, price_monthly = 3900.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-5';

UPDATE vps.plans
SET disk_gb = 250, price_monthly = 7900.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-6';
