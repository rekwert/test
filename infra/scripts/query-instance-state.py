#!/usr/bin/env python3
import os
import subprocess
import sys

hostname = sys.argv[1] if len(sys.argv) > 1 else "vps-90ej15"
dsn = os.environ.get("POSTGRES_DSN", "")
if not dsn:
    for line in open("/opt/testVPStrade/infra/docker/.env"):
        if line.startswith("POSTGRES_DSN="):
            dsn = line.split("=", 1)[1].strip()
            break

sql = f"""
SELECT i.hostname, i.state, i.external_id, i.ip_address::text, o.os_template_id
FROM vps.instances i
JOIN vps.orders o ON o.id = i.order_id
WHERE i.hostname = '{hostname}';
"""
subprocess.run(["psql", dsn, "-c", sql], check=True)
