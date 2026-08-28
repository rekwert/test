-- PROSTO-3 storage: 40 GB → 50 GB in all regions.
UPDATE vps.plans
SET disk_gb = 50
WHERE lower(COALESCE(tier, '')) = 'prosto'
  AND name = 'PROSTO-3';
