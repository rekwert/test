-- Midrange-6 storage → 150 GB in all regions.
UPDATE vps.plans
SET disk_gb = 150
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-6';
