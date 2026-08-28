# testVPStrade — платформа аренды VPS

Control plane для продажи/аренды VPS: клиентский портал, админка, Go-микросервисы, VirtFusion/Proxmox на backend (клиент VirtFusion не видит).

## Стек

| Слой | Технологии |
|------|------------|
| Backend | Go: gateway, auth, billing, vps, notification |
| Frontend | Next.js — портал + админка |
| Данные | PostgreSQL, Redis |
| Почта (локально) | Mailpit |
| Почта (prod) | Poste.io на отдельной Mail VPS |
| Гипервизор | VirtFusion → Proxmox (локально — mock) |

## Топология серверов

| № | Роль | Что крутится |
|---|------|--------------|
| 1 | Front | Traefik + Next.js |
| 2 | Back | API gateway + все Go-сервисы |
| 3 | DB | PostgreSQL + Redis |
| 4 | Mail | Poste.io |
| 5+ | Data plane | VirtFusion + Proxmox *(когда купите железо)* |

**Главная инструкция по prod:** [docs/deployment.md](docs/deployment.md) (развёртывание по машинам, env, чеклист)

**API для фронтенда:** [packages/api-spec/FRONTEND-HANDOFF.md](packages/api-spec/FRONTEND-HANDOFF.md)

Control plane готов к выкатке: auth, catalog, mock billing, order stub. VirtFusion — после железа.

## Быстрый старт (локально, Windows)

**Нужно:** Docker Desktop, Node.js (для fix-run.js)

```powershell
cd C:\Users\паштет\Projects\testVPStrade
copy .env.example .env
node scripts\fix-run.js
docker compose -f infra/docker/docker-compose.yml up -d --build
```

> **Windows / Cursor:** агент может сохранять файлы в UTF-16. Перед каждым `docker compose build` запускай `node scripts\fix-run.js`.

### URL локально

| Сервис | URL |
|--------|-----|
| Портал | http://localhost:3000 |
| Админка | http://localhost:3000/admin |
| API | http://localhost:8080/api/v1 |
| Health gateway | http://localhost:8080/health |
| Mailpit | http://localhost:8025 |

## Git и GitHub

```powershell
# Первый раз
git clone https://github.com/borishru-boop/testVPStrade.git
cd testVPStrade

# После изменений
node scripts\fix-run.js          # Windows: починить кодировку
git status
git add -A
git commit -m "описание изменений"
git push origin main
```

После push в `main` GitHub Actions собирает Docker-образы в GHCR. Подробнее: [docs/deploy-ci.md](docs/deploy-ci.md)

### Переменная в GitHub (для сборки web)

**Settings → Secrets and variables → Actions → Variables:**

| Имя | Пример |
|-----|--------|
| `NEXT_PUBLIC_API_URL` | `https://portal.yourdomain.com/api/v1` |

## Auth и почта (MVP)

- Регистрация / вход / JWT refresh
- Подтверждение email — код 6 цифр (RU + EN)
- Сброс пароля — код 6 цифр
- Цепочка: **auth** → HTTP → **notification** → SMTP → Mailpit (dev) или Poste.io (prod)

Страницы: `/login`, `/register`, `/verify-email`, `/forgot-password`, `/reset-password`

## Скрипты

| Скрипт | Назначение |
|--------|------------|
| `node scripts/fix-run.js` | UTF-16 → UTF-8 (Windows, перед билдом) |
| `scripts/rebuild-email.ps1` | Fix + rebuild auth, notification, web |
| `scripts/rebuild-auth.ps1` | Fix + rebuild auth, gateway, web |
| `infra/scripts/deploy-db.sh` | Деплой Machine 3 |
| `infra/scripts/deploy-back.sh` | Деплой Machine 2 |
| `infra/scripts/deploy-front.sh` | Деплой Machine 1 |
| `infra/scripts/deploy-mail.sh` | Деплой Machine 4 |
| `infra/scripts/docker-login-ghcr.sh` | Логин в GHCR на VPS |

## Документация

| Документ | О чём |
|----------|--------|
| **[Frontend handoff](packages/api-spec/FRONTEND-HANDOFF.md)** | **API для фронта: live vs mock, curl, fixtures** |
| **[Развёртывание](docs/deployment.md)** | **Env, compose, firewall, чеклист prod** |
| [CI/CD](docs/deploy-ci.md) | GitHub Actions, GHCR, IMAGE_TAG |
| [Архитектура](docs/architecture.md) | Общая схема |
| [Poste.io](docs/mail-poste.md) | DNS, DKIM, SMTP |

## Репозиторий

https://github.com/borishru-boop/testVPStrade
