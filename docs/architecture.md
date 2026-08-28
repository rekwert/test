# Architecture

## Overview

VPS hosting control plane with client portal (ONE DASH style), staff admin, and VirtFusion-backed provisioning.

## Topology

```
Internet
    |
Machine 1 (FRONT)     Traefik/Nginx + Next.js
    |                   (portal) + (admin)
    |
Machine 2 (BACK)      api-gateway
    |                   auth | billing | vps | notification
    |                   + workers (same machine, separate containers)
    |
Machine 3 (DB)        PostgreSQL (schemas: auth, billing, vps, notification)
    |                 Redis Streams
    |
Machine 4+ (later)    VirtFusion + Proxmox -> client VPS
```

## Services

| Service | Responsibility | Owns data |
|---|---|---|
| api-gateway | JWT, CORS, rate limit, routing | none |
| auth | register/login, RBAC, API keys | auth.* |
| billing | balance, invoices, grace period, webhooks | billing.* |
| vps | orders, instances, lifecycle, VirtFusion adapter | vps.* |
| notification | email/Telegram delivery | notification.* |

## Rules

- Clients use Portal only. No direct access to VirtFusion/VMmanager UI.
- VirtFusion is backend-only via adapter in vps-service.
- billing does NOT call VirtFusion. vps listens for payment events.
- No shared DB tables across service boundaries.
- Inter-service sync: REST (internal) + Redis Streams (async events).

## Client flow

```
Portal -> Gateway -> vps (create order)
                  -> billing (reserve/charge)
billing publishes payment.received
vps-worker provisions via VirtFusion adapter
Portal shows state machine updates
```

## Local development

Single docker-compose runs entire stack + virtfusion-mock + mailpit.

## Related docs

- [state-machine.md](state-machine.md)
- [events.md](events.md)
- [rbac.md](rbac.md)
- [deployment.md](deployment.md)
