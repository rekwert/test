-- NL VirtFusion max_servers capacity (IP pool / HV limit).
UPDATE vps.nodes
SET capacity_instances = 50,
    updated_at = now()
WHERE region = 'nl';
