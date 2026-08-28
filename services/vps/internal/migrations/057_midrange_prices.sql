-- Midrange monthly prices (all regions).
UPDATE vps.plans SET price_monthly = 475.00 WHERE lower(COALESCE(tier, '')) = 'midrange' AND name = 'Midrange-1';
UPDATE vps.plans SET price_monthly = 890.00 WHERE lower(COALESCE(tier, '')) = 'midrange' AND name = 'Midrange-2';
UPDATE vps.plans SET price_monthly = 1300.00 WHERE lower(COALESCE(tier, '')) = 'midrange' AND name = 'Midrange-3';
UPDATE vps.plans SET price_monthly = 1700.00 WHERE lower(COALESCE(tier, '')) = 'midrange' AND name = 'Midrange-4';
UPDATE vps.plans SET price_monthly = 2500.00 WHERE lower(COALESCE(tier, '')) = 'midrange' AND name = 'Midrange-5';
UPDATE vps.plans SET price_monthly = 4800.00 WHERE lower(COALESCE(tier, '')) = 'midrange' AND name = 'Midrange-6';
