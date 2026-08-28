-- FI hypervisor node accepts trial tier only (PROSTO stays on NL).

UPDATE vps.nodes
SET supported_tiers = ARRAY['trial']::text[]
WHERE region = 'fi'
  AND (name = 'FI-1' OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002')
  AND cardinality(supported_tiers) = 0;
