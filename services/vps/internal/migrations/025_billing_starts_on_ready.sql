-- Paid period ("Действует до") starts when the instance becomes ready, not at order time.
-- Clear premature next_billing_at for instances still provisioning.
UPDATE vps.instances
SET next_billing_at = NULL,
    updated_at = now()
WHERE state IN ('creating', 'queued')
  AND next_billing_at IS NOT NULL;
