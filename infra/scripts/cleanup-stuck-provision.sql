-- Cancel stuck NL provisioning so a fresh order can proceed.
UPDATE vps.outbox
SET published = true
WHERE event_type = 'instance.provision_requested'
  AND published = false;

UPDATE vps.instances
SET state = 'deleted',
    updated_at = now()
WHERE state = 'creating'
  AND region = 'nl';

SELECT id, hostname, state, external_id, host(ip_address)::text AS ip_address FROM vps.instances WHERE region = 'nl' ORDER BY created_at DESC LIMIT 10;
SELECT id, published, payload->>'hostname' AS hostname FROM vps.outbox WHERE event_type = 'instance.provision_requested' ORDER BY id;
