-- PROSTO-2 storage: 20 GB → 30 GB in all regions.
UPDATE vps.plans
SET disk_gb = 30
WHERE lower(COALESCE(tier, '')) = 'prosto'
  AND name = 'PROSTO-2';
