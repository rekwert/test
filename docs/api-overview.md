# API Overview (v1)

Base path: `/api/v1`

Full spec: `packages/api-spec/openapi.yaml`

## Auth

```
POST /auth/register
POST /auth/login
POST /auth/refresh
POST /auth/logout
GET  /auth/me
```

## Plans

```
GET    /plans
POST   /plans          (admin)
PUT    /plans/:id      (admin)
DELETE /plans/:id      (admin)
```

## Orders & Instances

```
POST   /orders
GET    /instances
GET    /instances/:id
POST   /instances/:id/start
POST   /instances/:id/stop
POST   /instances/:id/reboot
DELETE /instances/:id
POST   /instances/:id/addons
```

## Billing

```
GET  /billing/balance
POST /billing/topup
GET  /billing/invoices
POST /webhooks/payment
```

## Admin

```
GET /admin/users
GET /admin/instances
GET /admin/audit
```

## Phase 2 (not in MVP)

```
GET  /instances/:id/console
POST /instances/:id/password
POST /instances/:id/reinstall
GET  /instances/:id/snapshots
POST /instances/:id/snapshots
GET  /instances/:id/metrics
```
