-- Free-week PROSTO-1 uses a 7-day service period, not 30.
UPDATE vps.instances
SET billing_period_days = 7,
    updated_at = now()
WHERE (
    COALESCE((provider_meta->>'free_week')::boolean, false)
    OR COALESCE((provider_meta->>'trial')::boolean, false)
  )
  AND COALESCE((provider_meta->>'initial_prepaid_days')::int, 0) = 7
  AND billing_period_days <> 7;
