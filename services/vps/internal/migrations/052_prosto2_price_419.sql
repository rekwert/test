-- PROSTO-2 monthly price: 449 → 419 RUB (all regions).
UPDATE vps.plans
SET price_monthly = 419.00
WHERE name = 'PROSTO-2';
