-- Claude Code optional preinstall (Linux, Node 20+ capable images)

INSERT INTO vps.software_profiles (id, name, description, labels) VALUES
    (
      'claude-code',
      'Claude Code',
      'Claude Code CLI + web terminal (HTTP auth). Ubuntu 20.04+, Debian 11+, Alma/Rocky 8+.',
      '{"ru":"Claude Code (веб-терминал)","en":"Claude Code (web terminal)"}'::jsonb
    )
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    labels = EXCLUDED.labels;

INSERT INTO vps.os_software (os_id, software_id)
SELECT t.id, 'claude-code'
FROM vps.os_templates t
WHERE t.family IN ('debian', 'rhel')
  AND t.id NOT LIKE '%centos-7%'
  AND t.id NOT LIKE '%debian-9%'
  AND t.id NOT LIKE '%debian-10%'
  AND t.id NOT LIKE '%ubuntu-16%'
  AND t.id NOT LIKE '%ubuntu-18%'
ON CONFLICT DO NOTHING;
