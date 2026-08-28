-- NL hosts paid lines + trial. FI node stays trial-only for sale (see 029 / 032).
-- ListPlans still returns active FI paid plans with available=false for the full offer.

UPDATE vps.nodes
SET supported_tiers = ARRAY['trial', 'prosto', 'midrange', 'hustle']::text[]
WHERE region = 'nl';
