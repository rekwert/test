-- PROSTO-4 storage → 60 GB in all regions.
UPDATE vps.plans
SET disk_gb = 60
WHERE lower(COALESCE(tier, '')) = 'prosto'
  AND name = 'PROSTO-4';
