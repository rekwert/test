-- Retire PROSTO-6/7/8 in all regions; max Prosto tier is PROSTO-5.
UPDATE vps.plans
SET active = false
WHERE lower(COALESCE(tier, '')) = 'prosto'
  AND name IN ('PROSTO-6', 'PROSTO-7', 'PROSTO-8');
