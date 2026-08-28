-- Which product lines a hypervisor node may host (prosto / midrange / hustle / storage).
-- Empty array = sell nothing from this node. Capacity-full still allows waitlist.

ALTER TABLE vps.nodes
    ADD COLUMN IF NOT EXISTS supported_tiers text[] NOT NULL DEFAULT '{}';

ALTER TABLE vps.plans
    ADD COLUMN IF NOT EXISTS tier text NOT NULL DEFAULT '';

UPDATE vps.plans SET tier = 'prosto' WHERE lower(name) LIKE 'prosto%';
UPDATE vps.plans SET tier = 'midrange' WHERE lower(name) LIKE 'midrange%';
UPDATE vps.plans SET tier = 'hustle' WHERE lower(name) LIKE 'hustle%';
UPDATE vps.plans SET tier = 'storage' WHERE lower(name) LIKE 'storage%';

-- Seed NL-1 only if still unset (do not clobber admin edits on re-migrate).
UPDATE vps.nodes
SET supported_tiers = ARRAY['prosto']::text[]
WHERE (id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001' OR name = 'NL-1')
  AND cardinality(supported_tiers) = 0;
