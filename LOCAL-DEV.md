# Локальный стенд (Windows + Docker + опционально OpenStack)

Два уровня — можно начать с первого за ~10 минут без OpenStack.

## Уровень 1 — Portal + mock OpenStack (рекомендуется сначала)

Полный стек: Next.js portal, gateway, auth, billing, vps, worker, postgres, redis, mailpit.
VPS создаются через **mock-адаптер** (без реальных VM), billing в mock-режиме.

### Требования

- Docker Desktop (WSL2 backend)
- 8 GB RAM свободно
- Порты: 3000, 8080, 5432, 8025

### Запуск

```powershell
cd "d:\saas решение\testVPStrade-openstack-dev"
.\infra\scripts\local-dev-up.ps1
```

Первый запуск соберёт образы (~5–15 мин).

### URLs

| Сервис | URL |
|--------|-----|
| Portal | http://localhost:3000 |
| API | http://localhost:8080/api/v1 |
| Health | http://localhost:8080/health |
| Mailpit | http://localhost:8025 |

### Тестовый пользователь

```powershell
curl -X POST http://localhost:8080/api/v1/auth/register `
  -H "Content-Type: application/json" `
  -d '{"email":"dev@test.local","password":"Test1234!","locale":"ru"}'
```

Дальше в portal: пополнить баланс (mock billing) → заказать PROSTO-1 nl + ubuntu-22.04 → смотреть `vps-worker` logs.

```powershell
cd infra\docker
docker compose --env-file .env.local logs -f vps-worker
```

### Остановка

```powershell
.\infra\scripts\local-dev-down.ps1
```

---

## Уровень 2 — Реальный OpenStack в WSL (Sunbeam)

Для настоящих Nova VM. Тяжелее: **16 GB RAM** для WSL, 30–60 мин установки.

### 1. WSL Ubuntu

```powershell
wsl -d Ubuntu
```

В `.wslconfig` (Windows user profile) рекомендуется:

```ini
[wsl2]
memory=16GB
processors=4
```

### 2. Установка OpenStack

```bash
cd /mnt/d/saas\ решение/testVPStrade-openstack-dev
chmod +x infra/openstack/*.sh
./infra/openstack/wsl-sunbeam-install.sh
```

Скрипт ставит snap `openstack`, bootstrap Sunbeam, создаёт flavor/image и пишет `~/openstack-portal-dev.env`.

### 3. Подключить portal к OpenStack

Скопируйте значения из `~/openstack-portal-dev.env` в `infra/docker/.env.local`:

```env
OPENSTACK_USE_MOCK=false
OPENSTACK_AUTH_URL=http://host.docker.internal:5000/v3
...
```

Перезапуск portal:

```powershell
cd infra\docker
docker compose --env-file .env.local up -d --build vps vps-worker
```

Docker на Windows достучится до Keystone в WSL через `host.docker.internal` (WSL пробрасывает localhost).

### 4. Проверка OpenStack из WSL

```bash
source ~/demo-openrc
openstack server list
openstack hypervisor list
openstack network list
```

---

## Что увидите в portal

- Каталог тарифов (PROSTO/Midrange/Hustle) по регионам nl/fi/de/gb
- Заказ VPS → state `creating` → `running`
- Mock: фиктивный IP `10.66.x.x`, console/metrics из mock
- Real OpenStack: Nova UUID, Floating IP, noVNC console

---

## Troubleshooting

| Проблема | Решение |
|----------|---------|
| Docker not running | Запустить Docker Desktop, повторить `local-dev-up.ps1` |
| Port 5432 busy | Остановить локальный Postgres или убрать `ports` у postgres в compose |
| C: disk full | Go/cache на D:; Docker data folder на D: в Docker Desktop settings |
| OpenStack в WSL не ставится | Нужен KVM/nested virt или Multipass VM на Ubuntu; иначе оставайтесь на mock |
| Provision stuck | `docker compose logs vps-worker`; проверить `OPENSTACK_PLAN_MAP` / `OPENSTACK_OS_MAP` |

---

## Файлы

```
infra/docker/docker-compose.yml   — полный local stack
infra/docker/.env.local.example   — env шаблон
infra/scripts/local-dev-up.ps1    — старт
infra/openstack/wsl-sunbeam-install.sh — OpenStack в WSL
OPENSTACK-MIGRATION.md            — prod cutover checklist
```
