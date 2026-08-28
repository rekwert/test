-- Amnezia VPN optional preinstall (Ubuntu 24.04, Debian 12 only)

INSERT INTO vps.software_profiles (id, name, description, labels) VALUES
    (
      'amnezia',
      'Amnezia',
      'Amnezia VPN (Docker): AmneziaWG on UDP 443, vpn:// import link. Ubuntu 24.04 / Debian 12.',
      '{"ru":"Amnezia","en":"Amnezia"}'::jsonb
    )
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    labels = EXCLUDED.labels;

INSERT INTO vps.os_software (os_id, software_id)
SELECT t.id, 'amnezia'
FROM vps.os_templates t
WHERE t.id IN ('ubuntu-24.04', 'debian-12')
   OR t.id LIKE 'ubuntu-24.04%'
   OR t.id LIKE '%debian-12%'
ON CONFLICT DO NOTHING;
