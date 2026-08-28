# Frontend handoff — API integration

Document for frontend developers. No own hardware / VirtFusion required to start.

## Status summary

| Area | Status | Action for frontend |
|------|--------|---------------------|
| **Auth** | **LIVE** | Integrate now |
| **Plans / Orders / Instances** | **STUB** | Use mock JSON in `packages/api-spec/examples/` |
| **Billing / Admin** | **STUB** | Mock only |
| VirtFusion | Later | When hardware ready |

**Base URL (local):** `http://localhost:8080/api/v1`  
**Base URL (prod):** `https://portal.YOUR-DOMAIN/api/v1`

OpenAPI: `packages/api-spec/openapi.yaml`

---

## Run API locally

```powershell
cd testVPStrade
copy .env.example .env
node scripts\fix-run.js
docker compose -f infra/docker/docker-compose.yml up -d --build
```

| Service | URL |
|---------|-----|
| Portal | http://localhost:3000 |
| API | http://localhost:8080/api/v1 |
| Health | http://localhost:8080/health |
| Mailpit (email codes) | http://localhost:8025 |

---

## Auth (LIVE)

### Headers

```
Authorization: Bearer <access_token>
Content-Type: application/json
```

### Error format

```json
{ "error": "message" }
```

### User object

```json
{
  "id": "uuid",
  "email": "user@example.com",
  "roles": ["client"],
  "email_verified": false,
  "locale": "ru"
}
```

### Auth response (register / login / refresh)

See `packages/api-spec/examples/auth-register-response.json`

### Endpoints

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| POST | /auth/register | no | Sends verify email |
| POST | /auth/login | no | |
| POST | /auth/refresh | no | Body: refresh_token |
| GET | /auth/me | Bearer | |
| POST | /auth/verify-email | Bearer | Body: code (6 digits) |
| POST | /auth/resend-verification | Bearer | |
| POST | /auth/forgot-password | no | locale optional |
| POST | /auth/reset-password | no | email + code + password |

### Register

```json
POST /auth/register
{ "email": "user@test.com", "password": "Test1234!", "locale": "ru" }
```

- `locale`: `ru` | `en`
- **201** — tokens + user
- **409** — email already registered

### Verify email

```json
POST /auth/verify-email
Authorization: Bearer ...
{ "code": "123456" }
```

Response: `{ "message": "email verified", "user": { ... } }`

### Forgot / reset password

Forgot always returns (no email leak):

```json
{ "message": "if the email exists, a reset code was sent" }
```

Reset:

```json
POST /auth/reset-password
{ "email": "...", "code": "123456", "password": "NewPass123!" }
```

### Curl smoke test

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@test.com","password":"Test1234!","locale":"ru"}'
```

Check code in Mailpit: http://localhost:8025

---

## VPS / Billing / Admin (STUB)

Current response:

```json
{ "endpoint": "list instances", "status": "todo" }
```

Do **not** ship production UI against these endpoints yet.

Use fixtures:

| File | Screen |
|------|--------|
| `examples/plans-list.json` | Tariffs |
| `examples/instances-list.json` | Dashboard VPS list |
| `examples/order-catalog-mock.json` | Order wizard: OS + software |
| `examples/auth-register-response.json` | Auth shape |
| `examples/error-response.json` | Errors |

Future order body (draft):

```json
{
  "plan_id": "uuid",
  "region": "moscow",
  "hostname": "my-server-1",
  "os_template_id": "ubuntu-24.04",
  "software_profile_id": "docker-ce"
}
```

VPS states: `docs/state-machine.md`  
RBAC / buttons: `docs/rbac.md`

---

## Existing frontend reference

| Path | Notes |
|------|-------|
| /login, /register | Wired to API (draft UI) |
| /verify-email, /forgot-password, /reset-password | Wired |
| /dashboard | Skeleton, mock zeros |
| /admin | Skeleton |
| apps/web/src/lib/auth/session.ts | Auth API client |

You may replace all UI; session.ts is a reference.

---

## Work phases

| Phase | Can start now | API |
|-------|---------------|-----|
| 1 | Design system, landing, layout | — |
| 2 | Auth E2E | **LIVE** |
| 3 | Tariffs + order wizard | **Mock fixtures** |
| 4 | Dashboard, VPS card, power buttons | **Mock** |
| 5 | Billing, admin | **Mock** |

---

## CORS / i18n

- Prod Back env: `CORS_ORIGINS=https://portal.YOUR-DOMAIN`
- Local: localhost:3000 allowed
- Email templates: `locale` ru | en on register / forgot-password

---

## Repo

https://github.com/borishru-boop/testVPStrade

Deploy: `docs/deployment.md`
