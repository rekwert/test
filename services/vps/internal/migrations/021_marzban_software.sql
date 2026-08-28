-- Marzban Xray as optional software preinstall (Linux)

INSERT INTO vps.software_profiles (id, name, description, labels) VALUES
    (
      'marzban',
      'Marzban Xray',
      'Marzban panel (Xray). Linux only. Credentials: /root/info.txt',
      '{"ru":"Marzban Xray","en":"Marzban Xray"}'::jsonb
    )
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    labels = EXCLUDED.labels;

INSERT INTO vps.os_software (os_id, software_id)
SELECT t.id, 'marzban'
FROM vps.os_templates t
WHERE t.family IN ('debian', 'rhel')
ON CONFLICT DO NOTHING;
