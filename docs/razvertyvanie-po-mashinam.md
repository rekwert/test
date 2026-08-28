# Развёртывание по машинам (production)

Пошаговая инструкция: что на какой VPS, env-переменные, compose-файлы, firewall.

**Каждая VPS — свой Docker Compose. Связь по IP + порт.**

## Схема

```
Internet
    |
 [1 Front]  Traefik :443, Next.js
    |  /api/*
    v
 [2 Back]   gateway :8080, auth, billing, vps, notification
    |                    |
    +---> [3 DB] :5432, :6379
    +---> [4 Mail] :587 SMTP
    X---> [5+ VirtFusion] (pozzhe)
```

## Порядок развёртывания

| Шаг | Машина | Почему |
|-----|--------|--------|
| 1 | 3 DB | Без БД Back не стартует |
| 2 | 2 Back | API, миграции |
| 3 | 4 Mail | SMTP для кодов |
| 4 | 1 Front | Портал + HTTPS |
| 5 | 5+ | VirtFusion — когда железо |

```bash
git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
cd /opt/testVPStrade
```

---

## Machine 3 — DB (PostgreSQL + Redis)

| | |
|---|---|
| Железо | 4 vCPU, 32 GB RAM, 300+ GB NVMe |
| Compose | `infra/docker/docker-compose.db.yml` |
| Env | `cp infra/docker/.env.db.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-db.sh` |
| Бэкап | `bash infra/scripts/backup-db.sh` (cron) |

### Переменные .env

| Переменная | Пример | Описание |
|------------|--------|----------|
| `POSTGRES_USER` | vps | Пользователь PostgreSQL |
| `POSTGRES_PASSWORD` | *** | Пароль PostgreSQL |
| `POSTGRES_DB` | vps_platform | Имя базы |
| `REDIS_PASSWORD` | *** | Пароль Redis |
| `DB_BIND_IP` | 0.0.0.0 | IP для bind 5432/6379 |

### Firewall (UFW)

```bash
sudo ufw allow from BACK_VPS_IP to any port 5432 proto tcp
sudo ufw allow from BACK_VPS_IP to any port 6379 proto tcp
sudo ufw allow OpenSSH && sudo ufw enable
```

### Передать на Back

```env
POSTGRES_DSN=postgres://vps:PASS@DB_IP:5432/vps_platform?sslmode=disable
REDIS_URL=redis://:REDIS_PASS@DB_IP:6379/0
```

---

## Machine 2 — Back (API + workers)

| | |
|---|---|
| Железо | 4–6 vCPU, 16 GB RAM, 100 GB SSD |
| Compose | `infra/docker/docker-compose.back.yml` |
| Env | `cp infra/docker/.env.back.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-back.sh` |

### Переменные .env

| Переменная | Описание |
|------------|----------|
| `DB_VPS_IP` | IP Machine 3 |
| `POSTGRES_DSN` | Полная строка к Postgres |
| `REDIS_URL` | redis://:pass@DB_IP:6379/0 |
| `JWT_SECRET` | Длинная случайная строка |
| `CORS_ORIGINS` | https://portal.ваш-домен |
| `BACK_BIND_IP` | 0.0.0.0 |
| `GATEWAY_PORT` | 8080 |
| `SMTP_HOST` | mail.ваш-домен |
| `SMTP_PORT` | 587 |
| `SMTP_TLS` | true |
| `SMTP_USER` | noreply@ваш-домен |
| `SMTP_PASS` | пароль ящика Poste |
| `SMTP_FROM` | noreply@ваш-домен |
| `EMAIL_CODE_TTL` | 15m |
| `PASSWORD_RESET_TTL` | 15m |
| `VIRTFUSION_API_URL` | URL VirtFusion (пока mock) |
| `VIRTFUSION_API_KEY` | ключ VirtFusion |
| `BILLING_MOCK` | true |
| `IMAGE_TAG` | latest или sha из CI |
| `FRONT_VPS_IP` | IP Front (для UFW) |

### GHCR (перед docker pull)

```bash
export GHCR_USER=borishru-boop
export GHCR_TOKEN=ghp_xxxx
bash infra/scripts/docker-login-ghcr.sh
```

### Firewall

```bash
sudo ufw allow from FRONT_VPS_IP to any port 8080 proto tcp
sudo ufw allow out to DB_VPS_IP port 5432,6379 proto tcp
sudo ufw allow out to MAIL_VPS_IP port 587 proto tcp
```

### Сервисы

gateway :8080 (только Front) | auth :8001 | billing :8002 | vps :8003 | notification :8004

---

## Machine 4 — Mail (Poste.io)

| | |
|---|---|
| Железо | 2 vCPU, 4 GB RAM, 40 GB SSD |
| Compose | `infra/docker/docker-compose.mail.yml` |
| Env | `cp infra/docker/.env.mail.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-mail.sh` |

### Переменные .env

| Переменная | Описание |
|------------|----------|
| `POSTE_HOSTNAME` | mail.ваш-домен (A, PTR) |
| `TZ` | Europe/Moscow |
| `DISABLE_CLAMAV` | 1 — меньше RAM |
| `BACK_VPS_IP` | UFW: 587 только Back |
| `ADMIN_IP` | UFW: 443 admin только вы |
| `SMTP_USER` / `SMTP_FROM` | noreply@... |

### DNS

A mail | MX @ | SPF | DKIM (Poste admin) | DMARC | PTR

### После Poste

Обновить SMTP_* на Back → `docker compose -f docker-compose.back.yml up -d notification`

---

## Machine 1 — Front (Traefik + Next.js)

| | |
|---|---|
| Железо | 2 vCPU, 8 GB RAM, 60 GB SSD |
| Compose | `infra/docker/docker-compose.front.yml` |
| Env | `cp infra/docker/.env.front.example infra/docker/.env` |
| Деплой | `bash infra/scripts/deploy-front.sh` |

### Переменные .env

| Переменная | Описание |
|------------|----------|
| `DOMAIN` | portal.ваш-домен |
| `ACME_EMAIL` | Let's Encrypt |
| `BACK_GATEWAY_URL` | http://BACK_IP:8080 |
| `NEXT_PUBLIC_API_URL` | https://portal.ваш-домен/api/v1 |
| `IMAGE_TAG` | latest |

### Traefik

| Путь | Назначение |
|------|------------|
| / | Next.js :3000 |
| /api/* | Back gateway :8080 |

### Firewall: 80, 443 публично

**На Back:** `CORS_ORIGINS=https://portal.ваш-домен`

---

## Machine 5+ — VirtFusion

Proxmox + VirtFusion, VPN до Back, обновить `VIRTFUSION_*` в Back .env. VPS API сейчас заглушки — это норм.

---

## Сводка связей

| Откуда | Куда | Порт |
|--------|------|------|
| Internet | Front | 443 |
| Front | Back | 8080 |
| Back | DB | 5432, 6379 |
| Back | Mail | 587 |

## Compose и env-шаблоны

| Машина | Compose | Env example |
|--------|---------|-------------|
| 3 | docker-compose.db.yml | .env.db.example |
| 2 | docker-compose.back.yml | .env.back.example |
| 4 | docker-compose.mail.yml | .env.mail.example |
| 1 | docker-compose.front.yml | .env.front.example |
| local | docker-compose.yml | .env.example |

## Git / обновление

```bash
cd /opt/testVPStrade && git pull
bash infra/scripts/deploy-back.sh    # Back
bash infra/scripts/deploy-front.sh   # Front
```

CI/CD: [deploy-ci.md](./deploy-ci.md) | Чеклист: [infra-checklist.md](./infra-checklist.md)
