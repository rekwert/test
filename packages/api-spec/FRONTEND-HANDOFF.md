# Frontend handoff — API integration

For frontend developers. No VirtFusion hardware needed to start.

## Status

| Area | Status | Frontend action |
|------|--------|-----------------|
| Auth | LIVE | Integrate now |
| Plans + OS catalog | LIVE (test) | GET /plans, /catalog/os/.../software |
| Orders | LIVE (stub) | POST /orders validates + returns order id |
| Instances lifecycle | STUB | GET /instances returns [] |
| Billing | LIVE (mock) | balance=0, empty invoices |
| Admin | STUB | Not implemented |

Base URL local: http://localhost:8080/api/v1  
Base URL prod: https://portal.YOUR-DOMAIN/api/v1

OpenAPI: openapi.yaml in this folder.

## Run locally

```
copy .env.example .env
node scripts/fix-run.js
docker compose -f infra/docker/docker-compose.yml up -d --build
```

| Service | URL |
|---------|-----|
| Portal | http://localhost:3000 |
| API | http://localhost:8080/api/v1 |
| Mailpit | http://localhost:8025 |

## Auth (LIVE)

Header: `Authorization: Bearer ACCESS_TOKEN`

| Method | Path | Auth |
|--------|------|------|
| POST | /auth/register | no |
| POST | /auth/login | no |
| POST | /auth/refresh | no |
| GET | /auth/me | Bearer |
| POST | /auth/verify-email | Bearer |
| POST | /auth/resend-verification | Bearer |
| POST | /auth/forgot-password | no |
| POST | /auth/reset-password | no |

Register: `{"email","password","locale":"ru|en"}`  
Verify: `{"code":"123456"}`

## Catalog (LIVE)

| Method | Path |
|--------|------|
| GET | /plans |
| GET | /catalog |
| GET | /catalog/os |
| GET | /catalog/os/{os_id}/software |

Linux OS: clean, 3x-ui, python3. FreeBSD: clean, python3. NoOS: clean only.

## Orders (stub, LIVE validation)

POST /orders (Bearer required):

```json
{
  "plan_id": "11111111-1111-1111-1111-111111111101",
  "region": "moscow",
  "hostname": "my-vps",
  "os_template_id": "ubuntu-24.04",
  "software_profile_id": "3x-ui"
}
```

Response 201: order id + status pending. VirtFusion not wired yet.

## Billing (mock)

| Method | Path |
|--------|------|
| GET | /billing/balance |
| GET | /billing/invoices |
| POST | /billing/topup |

## Fixtures

| File | Use |
|------|-----|
| examples/plans-list.json | Tariff cards |
| examples/order-catalog-mock.json | OS wizard fallback |
| examples/instances-list.json | Dashboard empty state |
| examples/auth-register-response.json | Auth response shape |

Repo: https://github.com/borishru-boop/testVPStrade
