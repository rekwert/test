SELECT id, hostname, state, external_id, ip_address::text AS ip, created_at
FROM vps.instances
ORDER BY created_at DESC
LIMIT 8;

SELECT id, published, payload->>'hostname' AS hostname, payload->>'os_template_id' AS os_template
FROM vps.outbox
WHERE event_type = 'instance.provision_requested'
ORDER BY id DESC
LIMIT 5;
