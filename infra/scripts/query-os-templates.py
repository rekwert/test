#!/usr/bin/env python3
import os
import subprocess

dsn = os.environ.get("POSTGRES_DSN", "")
if not dsn:
    for line in open("/opt/testVPStrade/infra/docker/.env"):
        if line.startswith("POSTGRES_DSN="):
            dsn = line.split("=", 1)[1].strip()
            break

sql = """
SELECT id, name, external_version_id, active
FROM vps.os_templates
WHERE id LIKE 'ubuntu%' OR id LIKE 'centos%'
ORDER BY id;
"""
subprocess.run(["psql", dsn, "-c", sql], check=True)
