#!/usr/bin/env bash
set -euo pipefail
source /opt/testVPStrade/infra/docker/.env

USER_ID="e160cfad-6859-41e4-951f-2cedc17b6479"
PLAN_ID="11111111-1111-1111-1111-111111111101"
ORDER_ID="11111111-1111-1111-1111-111111111199"
INSTANCE_ID="11111111-1111-1111-1111-111111111198"
NODE_ID="bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001"

psql "$POSTGRES_DSN" <<SQL
\set ON_ERROR_STOP on
BEGIN;

UPDATE billing.accounts SET balance = balance - 149, updated_at = now() WHERE user_id = '$USER_ID'::uuid;

INSERT INTO vps.orders (id, user_id, plan_id, region, status, os_template_id, software_profile_id, hostname)
VALUES ('$ORDER_ID'::uuid, '$USER_ID'::uuid, '$PLAN_ID'::uuid, 'nl', 'paid', 'ubuntu-24.04', 'clean', 'vps-test');

INSERT INTO vps.instances (id, user_id, order_id, plan_id, region, node_id, state, billing_status, hostname, root_password, billing_period_days, next_billing_at)
VALUES ('$INSTANCE_ID'::uuid, '$USER_ID'::uuid, '$ORDER_ID'::uuid, '$PLAN_ID'::uuid, 'nl', '$NODE_ID'::uuid, 'creating', 'active', 'vps-test', 'KqyXBuHknwwxcTX7', 30, now() + ('30' || ' days')::interval);

INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id)
VALUES ('$USER_ID'::uuid, 149, 'paid', 'test', 'balance', 'charge', '$INSTANCE_ID'::uuid);

INSERT INTO vps.outbox (event_type, payload)
VALUES ('instance.provision_requested', '{"instance_id":"$INSTANCE_ID"}'::jsonb);

ROLLBACK;
SQL
echo "all inserts ok"
