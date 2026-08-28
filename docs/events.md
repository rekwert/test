# Event Catalog

All async communication uses Redis Streams. Each event has: `event_id`, `type`, `payload`, `timestamp`.

## Billing events

| Event | Publisher | Consumers | Payload |
|---|---|---|---|
| payment.received | billing | vps | order_id, user_id, amount |
| payment.failed | billing | notification | user_id, order_id, reason |
| balance.low | billing | notification | user_id, balance |

## VPS events

| Event | Publisher | Consumers | Payload |
|---|---|---|---|
| order.created | vps | billing, notification | order_id, plan_id, region |
| instance.provision_requested | vps | vps-worker | instance_id |
| instance.state_changed | vps | notification | instance_id, old_state, new_state |
| instance.suspend_requested | billing | vps | instance_id, reason |

## Notification events

| Event | Publisher | Consumers | Payload |
|---|---|---|---|
| notification.send_email | any | notification-worker | to, template, data |

## Idempotency

- All consumers check `processed_events(event_id, consumer)` before handling.
- Critical POST endpoints accept `Idempotency-Key` header.

## Outbox

billing and vps write outbox rows in same DB transaction as business data.
Workers poll outbox and publish to Redis Streams.
