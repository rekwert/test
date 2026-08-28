-- HUSTLE price ladder refresh (all regions).
UPDATE vps.plans SET price_monthly = 710.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-1';

UPDATE vps.plans SET price_monthly = 1350.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-2';

UPDATE vps.plans SET price_monthly = 2000.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-3';

UPDATE vps.plans SET price_monthly = 2650.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-4';

UPDATE vps.plans SET price_monthly = 3900.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-5';

UPDATE vps.plans SET price_monthly = 7900.00
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-6';
