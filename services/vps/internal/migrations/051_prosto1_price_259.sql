-- PROSTO-1 monthly price: 299 → 259 RUB (all regions).
UPDATE vps.plans
SET price_monthly = 259.00
WHERE name = 'PROSTO-1';
