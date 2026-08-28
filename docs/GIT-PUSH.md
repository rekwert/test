# Git push — testVPStrade (backend + infra)

Перед push на Windows:

```powershell
cd C:\Users\паштет\Projects\testVPStrade
node scripts\fix-run.js
git status
```

Коммит (`.env` не попадёт):

```powershell
git add -A
git status
git commit -m "Prod-ready: deploy scripts, gateway proxy, CI split front to FrontVPS repo"
git push origin main
```

После push: GitHub Actions → Docker Build (5 образов: gateway, auth, billing, vps, notification).

Front-образ собирается из репозитория **FrontVPS** → `testvps-trade-web`.

## Порядок первого деплоя на VPS

1. Push **testVPStrade** → дождаться CI
2. Push **FrontVPS** → дождаться CI (`testvps-trade-web`)
3. На VPS: Machine 3 → 2 → 4 → 1 (см. `docs/deployment.md`)

## GitHub Variables

**testVPStrade** — переменная `NEXT_PUBLIC_API_URL` больше не нужна для CI (web убран).

**FrontVPS** — обязательно:

`NEXT_PUBLIC_API_URL` = `https://portal.ваш-домен.ru/api/v1`

## GHCR на VPS

```bash
export GHCR_TOKEN=ghp_xxxx
bash infra/scripts/docker-login-ghcr.sh
```

В каждом `infra/docker/.env`: `IMAGE_TAG=latest`
