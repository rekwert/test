-- NL VirtFusion hypervisor id (control panel compute resource #1).
-- Replaces older SolusVM CR #2 binding; migrations re-run on every service start.

UPDATE vps.nodes
SET external_id = '1',
    status = 'online',
    vf_enabled = true,
    maintenance_mode = false,
    vf_commissioned = 3,
    updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001';
