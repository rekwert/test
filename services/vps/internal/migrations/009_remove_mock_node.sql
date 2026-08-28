-- Remove seed mock hypervisor (Node-1 / moscow); production uses VirtFusion nodes only.

UPDATE vps.instances
SET node_id = NULL, updated_at = now()
WHERE node_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01'::uuid;

DELETE FROM vps.nodes
WHERE id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01'::uuid;
