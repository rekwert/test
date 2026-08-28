-- OS catalog sync from SolusVM (external_version_id + seed static templates)

ALTER TABLE vps.os_templates
    ADD COLUMN IF NOT EXISTS external_version_id INT,
    ADD COLUMN IF NOT EXISTS synced_at TIMESTAMPTZ;

INSERT INTO vps.os_templates (id, name, version, family, active, sort_order) VALUES
    ('alma-8', 'Alma Linux', '8', 'rhel', false, 10),
    ('alma-9', 'Alma Linux', '9', 'rhel', false, 11),
    ('astra-ce', 'Astra Linux', 'CE', 'rhel', false, 12),
    ('centos-7', 'CentOS', '7', 'rhel', false, 13),
    ('centos-8-stream', 'CentOS', '8 Stream', 'rhel', false, 14),
    ('centos-9-stream', 'CentOS', '9 Stream', 'rhel', false, 15),
    ('debian-9', 'Debian', '9', 'debian', false, 20),
    ('debian-10', 'Debian', '10', 'debian', false, 21),
    ('debian-11', 'Debian', '11', 'debian', false, 22),
    ('debian-12', 'Debian', '12', 'debian', false, 23),
    ('freebsd-13', 'FreeBSD', '13', 'freebsd', false, 30),
    ('noos', 'NoOS', '', 'none', false, 99),
    ('oracle-8', 'Oracle Linux', '8', 'rhel', false, 40),
    ('oracle-9', 'Oracle Linux', '9', 'rhel', false, 41),
    ('rocky-8', 'Rocky Linux', '8', 'rhel', false, 42),
    ('ubuntu-16.04', 'Ubuntu', '16.04', 'debian', false, 50),
    ('ubuntu-18.04', 'Ubuntu', '18.04', 'debian', false, 51),
    ('ubuntu-20.04', 'Ubuntu', '20.04 LTS', 'debian', false, 52),
    ('ubuntu-22.04', 'Ubuntu', '22.04 LTS', 'debian', false, 53),
    ('ubuntu-24.04', 'Ubuntu', '24.04 LTS', 'debian', false, 54)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vps.os_software (os_id, software_id)
SELECT t.id, s.id
FROM vps.os_templates t
CROSS JOIN vps.software_profiles s
WHERE s.id IN ('clean', '3x-ui', 'python3')
  AND t.family IN ('rhel', 'debian')
ON CONFLICT DO NOTHING;

INSERT INTO vps.os_software (os_id, software_id)
SELECT 'freebsd-13', 'clean'
ON CONFLICT DO NOTHING;

INSERT INTO vps.os_software (os_id, software_id)
SELECT 'freebsd-13', 'python3'
ON CONFLICT DO NOTHING;

INSERT INTO vps.os_software (os_id, software_id)
SELECT 'noos', 'clean'
ON CONFLICT DO NOTHING;
