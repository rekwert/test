-- Retire Midrange-7 in all regions; Midrange-6 storage → 120 GB.
UPDATE vps.plans
SET active = false
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-7';

UPDATE vps.plans
SET disk_gb = 120
WHERE lower(COALESCE(tier, '')) = 'midrange'
  AND name = 'Midrange-6';
