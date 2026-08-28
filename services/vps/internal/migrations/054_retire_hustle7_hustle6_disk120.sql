-- Retire HUSTLE-7 in all regions; HUSTLE-6 storage → 120 GB.
UPDATE vps.plans
SET active = false
WHERE lower(COALESCE(tier, '')) = 'hustle'
  AND name = 'HUSTLE-7';

UPDATE vps.plans
SET disk_gb = 120
WHERE lower(COALESCE(tier, '')) = 'hustle'
  AND name = 'HUSTLE-6';
