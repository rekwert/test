# Миграция сервера БД (Machine 3) на новый IP

Безопасный перенос PostgreSQL + Redis с минимальным downtime.

## Текущая prod-схема (108.174.78.39)

| Параметр | Значение |
|----------|----------|
| Public IP | `108.174.78.39` |
| Private IP (Selectel LAN) | `192.168.0.5` |
| Back VPS public | `198.13.189.75` |
| Back VPS private | `192.168.0.2` |
| Compose | `/opt/db/docker-compose.yml` (не `/opt/testVPStrade`) |
| Postgres | `postgres:16-alpine`, порт **5432**, без PgBouncer |
| Redis | порт **6379**, AOF |
| UFW | 5432/6379 только с `198.13.189.75` (public Back) |
| Back `.env` | `DB_VPS_IP=108.174.78.39`, `sslmode=disable` |

**Рекомендация:** на новом сервере перейти на private IP `192.168.0.x` + TLS + PgBouncer (шаблон в `infra/docker/docker-compose.db.yml`).

---

## Фаза 0 — подготовка (без downtime)

### 1. Аудит текущего сервера

```bash
# На DB (108.174.78.39)
git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
bash /opt/testVPStrade/infra/scripts/db-migration-precheck.sh
```

```bash
# На Back (198.13.189.75)
ROLE=back bash /opt/testVPStrade/infra/scripts/db-migration-precheck.sh
```

### 2. Бэкап (обязательно до любых работ)

```bash
# На DB — через docker (текущий prod)
bash /opt/testVPStrade/infra/scripts/db-migration-backup.sh
```

Cron (ежедневно 03:00):

```bash
0 3 * * * /opt/testVPStrade/infra/scripts/db-migration-backup.sh >> /var/log/testVPStrade-backup.log 2>&1
```

Скопировать последний дамп off-site (scp на Back или S3).

### 3. Новый сервер БД

Минимум: 4 vCPU, 32 GB RAM, 100+ GB NVMe, Ubuntu 22.04/24.04.

```bash
apt update && apt upgrade -y
apt install -y docker.io docker-compose-v2 ufw git postgresql-client
systemctl enable --now docker

git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
cp infra/docker/.env.db.example infra/docker/.env
nano infra/docker/.env   # пароли как на старом сервере!
```

**Важно:** пароли Postgres/Redis должны совпадать со старым сервером, иначе при cutover придётся менять `.env` на Back.

`BACK_VPS_IP` в `.env` — private IP Back (`192.168.0.2`). Если Back подключается по public IP — добавить `BACK_VPS_PUBLIC_IP=198.13.189.75`.

```bash
bash infra/scripts/deploy-db.sh
```

### 4. UFW на новом сервере

```bash
BACK_VPS_IP=192.168.0.2 BACK_VPS_PUBLIC_IP=198.13.189.75 \
  bash infra/scripts/harden-db-ufw.sh
```

Разрешить: 5432, 6432 (PgBouncer), 6379 — **только** с Back.

### 5. Тестовое восстановление (на новом сервере, staging)

```bash
bash infra/scripts/db-migration-restore.sh /var/backups/testVPStrade/vps_platform_YYYYMMDD_HHMMSS.sql.gz
bash infra/scripts/db-migration-precheck.sh
```

---

## Фаза 1 — cutover (короткий downtime ~5–15 мин)

### Порядок

1. **Объявить maintenance** — остановить новые заказы (опционально: `docker stop docker-vps-worker-1` на Back).
2. **Финальный бэкап** на старом DB:
   ```bash
   bash /opt/testVPStrade/infra/scripts/db-migration-backup.sh /var/backups/testVPStrade/vps_platform_final.sql.gz
   ```
3. **Остановить Back stack** (чтобы не писал в старую БД):
   ```bash
   cd /opt/testVPStrade/infra/docker
   docker compose -f docker-compose.back.yml stop
   ```
4. **Restore на новый сервер** (если не sync/replica):
   ```bash
   scp root@108.174.78.39:/var/backups/testVPStrade/vps_platform_final.sql.gz /tmp/
   bash infra/scripts/db-migration-restore.sh /tmp/vps_platform_final.sql.gz
   ```
5. **Обновить Back `.env`** (`/opt/testVPStrade/infra/docker/.env`):
   ```env
   DB_VPS_IP=NEW_PRIVATE_IP          # предпочтительно 192.168.0.x
   POSTGRES_DSN=postgres://vps:PASSWORD@NEW_IP:6432/vps_platform?sslmode=require
   MIGRATION_DSN=postgres://vps:PASSWORD@NEW_IP:5432/vps_platform?sslmode=require
   REDIS_URL=redis://:REDIS_PASSWORD@NEW_IP:6379/0
   POSTGRES_SSL_ALLOW_INSECURE=false
   ```
6. **UFW на новом DB** — проверить правила для Back IP.
7. **Precheck с Back:**
   ```bash
   ROLE=back bash /opt/testVPStrade/infra/scripts/db-migration-precheck.sh
   ```
8. **Запустить Back:**
   ```bash
   bash /opt/testVPStrade/infra/scripts/deploy-back.sh
   ```
9. **Smoke test:** login, balance, список серверов, admin stats.
10. **Старый сервер** — оставить выключенным 24–48 ч (не удалять данные сразу).

### Rollback

Если что-то пошло не так:

1. Вернуть в Back `.env` старые `DB_VPS_IP`, `POSTGRES_DSN`, `REDIS_URL`.
2. Запустить postgres/redis на старом сервере (если останавливали).
3. `deploy-back.sh`.

---

## Чеклист после миграции

- [ ] Back health: `curl http://192.168.0.2:8080/health`
- [ ] Миграции: все `067_*` applied в `platform.schema_migrations`
- [ ] Cron backup на **новом** DB
- [ ] UFW: 5432/6379 не открыты в интернет
- [ ] Пароли не в git / не в plain-text compose (перенести в `.env`)
- [ ] VirtFusion NL `.env.purge` (если есть) — обновить `POSTGRES_DSN`
- [ ] Мониторинг node-exporter / алерты — новый IP

---

## Файлы, где меняется IP БД

| Где | Что |
|-----|-----|
| Back `/opt/testVPStrade/infra/docker/.env` | `DB_VPS_IP`, `POSTGRES_DSN`, `MIGRATION_DSN`, `REDIS_URL` |
| DB `.env` | `BACK_VPS_IP`, `BACK_VPS_PUBLIC_IP` |
| UFW на DB | `harden-db-ufw.sh` |
| NL VirtFusion (опционально) | `.env.purge` |

Код приложений IP не хардкодит — только env.

---

## SMAS на том же сервере

На `108.174.78.39` также крутится **SMAS** (`/opt/smas/`, отдельный postgres). Миграция `vps_platform` **не затрагивает** SMAS. Если переносите весь VPS — SMAS нужно мигрировать отдельно или оставить на старом хосте.
