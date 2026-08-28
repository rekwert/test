# Развёртывание — обзор и инструкция по машинам

> **Главный документ:** этот файл + детальные гайды `deploy-machine-*.md`  
> **Чеклист:** раздел «Чеклист prod» ниже

## Машины

| № | Роль | vCPU | RAM | Диск |
|---|------|------|-----|------|
| 1 | Front — Traefik + Next.js | 2 | 8 GB | 60 GB |
| 2 | Back — gateway + сервисы | 4–6 | 16 GB | 100 GB |
| 3 | DB — PostgreSQL + Redis | 4–8 | 32 GB | 300+ GB NVMe |
| 4 | Mail — Poste.io | 2 | 4 GB | 40 GB |
| 5+ | VirtFusion + Proxmox | — | — | — |

## Порядок: 3 → 2 → 4 → 1 → 5+

Каждая VPS — свой Docker Compose. Связь по **IP + порт**.

```
Internet -> [1 Front :443] -> /api -> [2 Back :8080] -> [3 DB] [4 Mail :587]
```

---

## Machine 3 — DB

| | |
|---|---|
| Compose | `infra/docker/docker-compose.db.yml` |
| Env | `cp infra/docker/.env.db.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-db.sh` |

**Env:** `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `REDIS_PASSWORD`, `DB_BIND_IP`

**UFW:** 5432, 6379 — только Back IP

**На Back:** `POSTGRES_DSN`, `REDIS_URL`

Детали: [deploy-machine-3-db.md](./deploy-machine-3-db.md)

---

## Machine 2 — Back

| | |
|---|---|
| Compose | `infra/docker/docker-compose.back.yml` |
| Env | `cp infra/docker/.env.back.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-back.sh` |

**Env (главные):**

| Переменная | Описание |
|------------|----------|
| POSTGRES_DSN | postgres://...@DB_IP:5432/vps_platform |
| REDIS_URL | redis://:pass@DB_IP:6379/0 |
| JWT_SECRET | секрет JWT |
| CORS_ORIGINS | https://portal.ваш-домен |
| GATEWAY_PORT | 8080 |
| SMTP_HOST/PORT/TLS/USER/PASS/FROM | Poste.io |
| VIRTFUSION_API_URL/KEY | позже |
| BILLING_MOCK | true |
| IMAGE_TAG | latest (GHCR) |

**GHCR:** `bash infra/scripts/docker-login-ghcr.sh`

**UFW:** 8080 — только Front IP

Детали: [deploy-machine-2-back.md](./deploy-machine-2-back.md)

---

## Machine 4 — Mail

| | |
|---|---|
| Compose | `infra/docker/docker-compose.mail.yml` |
| Env | `cp infra/docker/.env.mail.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-mail.sh` |

**Env:** `POSTE_HOSTNAME`, `TZ`, `DISABLE_CLAMAV`, `BACK_VPS_IP`, `ADMIN_IP`

**DNS:** A mail, MX, SPF, DKIM, DMARC, PTR

После Poste: SMTP_* на Back → `up -d notification`

Детали: [deploy-machine-4-mail.md](./deploy-machine-4-mail.md), [mail-poste.md](./mail-poste.md)

---

## Machine 1 — Front

| | |
|---|---|
| Compose | `infra/docker/docker-compose.front.yml` |
| Env | `cp infra/docker/.env.front.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-front.sh` |

**Env:** `DOMAIN`, `ACME_EMAIL`, `BACK_GATEWAY_URL`, `NEXT_PUBLIC_API_URL`, `IMAGE_TAG`

**Traefik:** `/` → Next.js, `/api/*` → Back

**UFW:** 80, 443 публично. **Back:** CORS_ORIGINS = https://portal.домен

Детали: [deploy-machine-1-front.md](./deploy-machine-1-front.md)

---

## Machine 5+ VirtFusion

Обновить `VIRTFUSION_*` на Back когда железо готово. VPS API — заглушки, это OK.

---

## Compose и env-шаблоны

| Машина | Compose | Env |
|--------|---------|-----|
| 3 | docker-compose.db.yml | .env.db.example |
| 2 | docker-compose.back.yml | .env.back.example |
| 4 | docker-compose.mail.yml | .env.mail.example |
| 1 | docker-compose.front.yml | .env.front.example |
| local | docker-compose.yml | .env.example |

---

## CI/CD

[deploy-ci.md](./deploy-ci.md) — GitHub Actions, GHCR, IMAGE_TAG

---

## Git

```powershell
node scripts\fix-run.js
git add -A
git commit -m "описание"
git push origin main
```

На VPS: `git pull` → `deploy-*.sh`

---

## Чеклист prod

Backend готов к выкатке control plane (auth + catalog + mock billing/orders). VirtFusion — после железа.

### Купить
- [ ] 4 VPS (Front, Back, DB, Mail) + домен
- [ ] Позже: Proxmox/VirtFusion

### DNS
- [ ] portal A, mail A, MX, SPF, DKIM, DMARC, PTR

### Секреты
- [ ] Пароли DB/Redis/JWT/SMTP, GHCR PAT
- [ ] .env не в git

### Firewall
- [ ] DB ← Back only; Back ← Front only; Mail ← Back on 587

### GitHub
- [ ] CI green (testVPStrade: 5 images; FrontVPS: testvps-trade-web)
- [ ] FrontVPS variable `NEXT_PUBLIC_API_URL=https://portal.домен/api/v1`

### Smoke
- [ ] Регистрация → код в почте → вход

### Backup
- [ ] cron backup-db.sh на DB

---

## Связанные документы

| Документ | Содержание |
|----------|------------|
| [deploy-ci.md](./deploy-ci.md) | CI/CD |
| [deploy-machine-3-db.md](./deploy-machine-3-db.md) | DB детали |
| [deploy-machine-2-back.md](./deploy-machine-2-back.md) | Back детали |
| [deploy-machine-4-mail.md](./deploy-machine-4-mail.md) | Mail детали |
| [deploy-machine-1-front.md](./deploy-machine-1-front.md) | Front детали |
