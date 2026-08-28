-- HUSTLE-1/2/3 storage → 60 GB in all regions.
UPDATE vps.plans
SET disk_gb = 60
WHERE lower(COALESCE(tier, '')) = 'hustle'
  AND name IN ('HUSTLE-1', 'HUSTLE-2', 'HUSTLE-3');
