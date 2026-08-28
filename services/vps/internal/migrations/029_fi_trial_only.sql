-- FI hypervisor is trial-only; PROSTO and other paid tiers stay on NL.

UPDATE vps.nodes
SET supported_tiers = ARRAY['trial']::text[]
WHERE region = 'fi'
  AND (name = 'FI-1' OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002');

UPDATE vps.plans
SET active = false
WHERE region = 'fi'
  AND tier = 'prosto';
