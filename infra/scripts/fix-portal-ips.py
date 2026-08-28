#!/usr/bin/env python3
"""Sync portal DB instance rows with SolusVM (one-off repair tool)."""
import os
import sys

try:
    import psycopg2
except ImportError:
    print("pip install psycopg2-binary", file=sys.stderr)
    raise

dsn = os.environ.get("POSTGRES_DSN")
if not dsn:
    print("set POSTGRES_DSN=postgres://user:pass@host:5432/vps_platform?sslmode=disable", file=sys.stderr)
    sys.exit(1)

# hostname → (external_id, primary_ip). Edit before use.
mapping: dict[str, tuple[str, str]] = {
    # "vps-example": ("165", "66.248.206.61"),
}

conn = psycopg2.connect(dsn)
conn.autocommit = True
cur = conn.cursor()

cur.execute(
    "SELECT id, hostname, external_id, host(ip_address)::text AS ip_address, state FROM vps.instances "
    "WHERE state != 'deleted' ORDER BY created_at DESC LIMIT 15"
)
print("BEFORE:")
for row in cur.fetchall():
    print(row)

for hostname, (ext_id, ip) in mapping.items():
    cur.execute(
        "UPDATE vps.instances SET external_id = %s, ip_address = %s::inet, state = 'running' "
        "WHERE hostname = %s AND state != 'deleted'",
        (ext_id, ip, hostname),
    )
    print("updated", hostname, cur.rowcount)

cur.execute(
    "UPDATE vps.outbox SET status = 'published', processed_at = now() "
    "WHERE status != 'published'"
)
print("outbox published:", cur.rowcount)

cur.execute(
    "SELECT id, hostname, external_id, host(ip_address)::text AS ip_address, state FROM vps.instances "
    "WHERE state != 'deleted' ORDER BY created_at DESC LIMIT 10"
)
print("AFTER:")
for row in cur.fetchall():
    print(row)

cur.close()
conn.close()
