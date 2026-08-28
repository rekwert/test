-- PROSTO-5: 6 vCPU -> 4 vCPU (all regions).
UPDATE vps.plans
SET cpu = 4
WHERE lower(COALESCE(tier, '')) = 'prosto' AND name = 'PROSTO-5';
