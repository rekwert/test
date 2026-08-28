-- Retire Trial SKUs; free week moves to PROSTO-1 (application logic).
UPDATE vps.plans
SET active = false
WHERE lower(COALESCE(tier, '')) = 'trial'
   OR lower(name) LIKE 'trial%';

-- Drop trial from node allow-lists.
UPDATE vps.nodes
SET supported_tiers = array_remove(supported_tiers, 'trial')
WHERE supported_tiers IS NOT NULL
  AND 'trial' = ANY (supported_tiers);
