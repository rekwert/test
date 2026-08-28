-- NL VirtFusion hypervisor node (Hostkey 66.248.206.14)

INSERT INTO vps.nodes (id, name, region, external_id, status, capacity_instances)
VALUES (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001',
    'NL-1',
    'nl',
    '1',
    'online',
    30
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    region = EXCLUDED.region,
    external_id = EXCLUDED.external_id,
    status = EXCLUDED.status,
    capacity_instances = EXCLUDED.capacity_instances;
