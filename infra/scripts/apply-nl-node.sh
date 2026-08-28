#!/bin/bash
source /opt/testVPStrade/infra/docker/.env
psql "$POSTGRES_DSN" -f /tmp/007_nl_node.sql
psql "$POSTGRES_DSN" -At -c "SELECT id,name,region,status FROM vps.nodes"
