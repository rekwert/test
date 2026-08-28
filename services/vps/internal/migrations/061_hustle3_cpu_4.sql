-- HUSTLE-3: 2 vCPU -> 4 vCPU (all regions).
UPDATE vps.plans
SET cpu = 4
WHERE lower(COALESCE(tier, '')) = 'hustle' AND name = 'HUSTLE-3';
