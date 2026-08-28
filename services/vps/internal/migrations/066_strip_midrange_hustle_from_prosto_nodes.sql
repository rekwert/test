-- Prosto hypervisors must not claim Midrange/Hustle (admin UI may re-add them).
UPDATE vps.nodes
SET supported_tiers = COALESCE((
    SELECT array_agg(t ORDER BY t)
    FROM unnest(supported_tiers) AS t
    WHERE t NOT IN ('midrange', 'hustle')
), ARRAY[]::text[])
WHERE 'prosto' = ANY(supported_tiers)
  AND supported_tiers && ARRAY['midrange', 'hustle']::text[];
