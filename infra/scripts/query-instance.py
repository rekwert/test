#!/usr/bin/env python3
import os
import sys

hostname = sys.argv[1] if len(sys.argv) > 1 else "vps-90ej15"
dsn = os.environ.get("POSTGRES_DSN", "")
if not dsn:
    env_path = "/opt/testVPStrade/infra/docker/.env"
    if os.path.isfile(env_path):
        for line in open(env_path):
            if line.startswith("POSTGRES_DSN="):
                dsn = line.split("=", 1)[1].strip()
                break

import subprocess

sql = f"""
SELECT i.hostname, p.name, i.plan_id::text, o.os_template_id, i.external_id,
       char_length(i.root_password) AS pwd_len
FROM vps.instances i
JOIN vps.plans p ON p.id = i.plan_id
JOIN vps.orders o ON o.id = i.order_id
WHERE i.hostname = '{hostname}';
"""
subprocess.run(["psql", dsn, "-c", sql], check=True)
