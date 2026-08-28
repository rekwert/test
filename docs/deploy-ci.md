# CI/CD и деплой Docker

## Git (локально → GitHub)

Все изменения кода коммитятся и пушатся в `main`:

| Репозиторий | Содержимое |
|-------------|------------|
| **testVPStrade** | backend, gateway, infra, deploy-скрипты |
| **FrontVPS** | портал (Next.js) |

## Почему на проде были «старые» образы

1. GitHub Actions **не собирал** образы автоматически (`workflow_dispatch` only).
2. Деплой делал `docker compose pull` → тянул **устаревший** `:latest` из GHCR.
3. Код в `/opt/testVPStrade` обновлялся, а контейнер — нет.

## Рекомендуемый деплой (всегда актуальный код)

На серверах образы **собираются из git**, а не pull из registry.

### Back VPS (`198.13.189.75`)

```bash
bash /opt/testVPStrade/infra/scripts/deploy-prod-back-now.sh
```

Или явно:

```bash
cd /opt/testVPStrade
git pull origin main
BUILD_LOCAL=1 bash infra/scripts/deploy-back.sh
```

`BUILD_LOCAL=1` (по умолчанию) → `build-back-images.sh` собирает gateway, auth, billing, **vps**, notification, support из текущего дерева git.

### Front VPS (`213.148.3.172`)

```bash
bash /opt/testVPStrade/infra/scripts/deploy-prod-front-now.sh
```

Или:

```bash
cd /opt/testVPStrade && git pull origin main
BUILD_LOCAL=1 bash infra/scripts/deploy-front.sh
```

`BUILD_LOCAL=1` (по умолчанию) → `build-front-image.sh` обновляет `/opt/FrontVPS` и собирает web-образ.

### Pull из GHCR (только если CI уже собрал свежий образ)

```bash
BUILD_LOCAL=0 IMAGE_TAG=latest bash infra/scripts/deploy-back.sh
```

Используйте **только** если уверены, что в GHCR лежит образ от последнего коммита.

## GitHub Actions

| Workflow | Когда | Что собирает |
|----------|-------|--------------|
| `docker-build-vps.yml` | push в `main` (paths: `services/vps/**`) | только **vps** → GHCR |
| `docker-build.yml` | вручную | все 7 backend-образов |
| FrontVPS `docker-build.yml` | push в `main` | **web** → GHCR |

GHCR образы — **резерв/кэш**. Прод по умолчанию собирает локально (`BUILD_LOCAL=1`).

## Переменные

- `BUILD_LOCAL=1` — собрать на сервере из git (рекомендуется)
- `BUILD_LOCAL=0` — pull из GHCR
- `NO_CACHE=1` — полная пересборка без кэша Docker
- `FRONT_ROOT=/opt/FrontVPS` — путь к репозиторию портала на front-сервере

## GitHub Variable (FrontVPS CI)

Settings → Actions → Variables: `NEXT_PUBLIC_API_URL` = `https://cloud-hustle.com/api/v1`
