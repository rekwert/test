-- Canonical tier assignments per physical hypervisor.
-- DE-1: hustle only (no prosto). DE-mid: midrange only.

UPDATE vps.nodes
SET supported_tiers = ARRAY['hustle']::text[], updated_at = now()
WHERE name = 'DE-1'
  OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003'::uuid;

UPDATE vps.nodes
SET supported_tiers = ARRAY['midrange']::text[], updated_at = now()
WHERE name = 'DE-mid'
  OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005'::uuid;

UPDATE vps.nodes
SET supported_tiers = ARRAY['prosto']::text[], updated_at = now()
WHERE name = 'FI-1'
  OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002'::uuid;

UPDATE vps.nodes
SET supported_tiers = ARRAY['prosto', 'midrange', 'hustle']::text[], updated_at = now()
WHERE name = 'GB-1'
  OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb004'::uuid;

UPDATE vps.nodes
SET supported_tiers = ARRAY['prosto', 'midrange']::text[], updated_at = now()
WHERE name = 'NL-1'
  OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001'::uuid;
