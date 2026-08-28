-- Sellable product lines per GEO (admin toggles). Node supported_tiers remain hardware routing.

CREATE TABLE IF NOT EXISTS vps.region_tiers (
    region     text NOT NULL REFERENCES vps.regions (code) ON DELETE CASCADE,
    tier       text NOT NULL,
    enabled    boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (region, tier),
    CONSTRAINT region_tiers_tier_check CHECK (
        tier = ANY (ARRAY['prosto', 'midrange', 'hustle', 'custom']::text[])
    )
);

INSERT INTO vps.region_tiers (region, tier, enabled)
SELECT r.code, t.tier, false
FROM vps.regions r
CROSS JOIN (
    VALUES ('prosto'), ('midrange'), ('hustle'), ('custom')
) AS t(tier)
ON CONFLICT (region, tier) DO NOTHING;

UPDATE vps.region_tiers rt
SET enabled = true,
    updated_at = now()
FROM (
    SELECT DISTINCT n.region, lower(trim(u.tier)) AS tier
    FROM vps.nodes n
    CROSS JOIN LATERAL unnest(n.supported_tiers) AS u(tier)
    WHERE n.status = 'online'
      AND n.external_id IS NOT NULL AND n.external_id <> ''
      AND COALESCE(n.vf_enabled, true) = true
      AND COALESCE(n.maintenance_mode, false) = false
      AND trim(u.tier) <> ''
) live
WHERE rt.region = live.region
  AND rt.tier = live.tier;

-- Canonical DE hardware split (infra, not sold via node UI).
UPDATE vps.nodes
SET supported_tiers = ARRAY['prosto']::text[], updated_at = now()
WHERE name = 'DE-1'
   OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003'::uuid;

UPDATE vps.nodes
SET supported_tiers = ARRAY['midrange']::text[], updated_at = now()
WHERE name = 'DE-mid'
   OR id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb005'::uuid;
