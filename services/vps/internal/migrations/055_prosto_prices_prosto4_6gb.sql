-- PROSTO price refresh + PROSTO-4 RAM 6 GB (all regions).
UPDATE vps.plans
SET price_monthly = 215.00
WHERE name = 'PROSTO-1';

UPDATE vps.plans
SET price_monthly = 380.00
WHERE name = 'PROSTO-2';

UPDATE vps.plans
SET price_monthly = 690.00
WHERE name = 'PROSTO-3';

UPDATE vps.plans
SET ram_mb = 6144,
    price_monthly = 990.00
WHERE name = 'PROSTO-4';

UPDATE vps.plans
SET price_monthly = 1300.00
WHERE name = 'PROSTO-5';
