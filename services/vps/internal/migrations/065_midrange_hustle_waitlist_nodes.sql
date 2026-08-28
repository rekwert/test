-- Prosto-line hypervisors must not claim Midrange/Hustle until dedicated nodes exist.
UPDATE vps.nodes
SET supported_tiers = COALESCE((
    SELECT array_agg(t ORDER BY t)
    FROM unnest(supported_tiers) AS t
    WHERE t NOT IN ('midrange', 'hustle')
), ARRAY[]::text[])
WHERE supported_tiers && ARRAY['midrange', 'hustle']::text[];
