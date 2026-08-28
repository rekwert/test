# Telegram-бот Cloud-hustle

Сервис `services/telegram-bot` работает на **машине Back** (рядом с auth/vps/billing).

## Возможности (без рефералки)

- Главное меню
- Тарифы (кратко) + ссылка «Заказать на сайте»
- Мои серверы → карточка → перезагрузка / доступы / баланс
- Баланс / пополнение (ссылка на оплату)
- Поддержка (ссылка в ЛК)
- Канал

## Привязка аккаунта

### Из бота
1. «Привязать аккаунт» → email  
2. На почту приходит код (`telegram_link`)  
3. Код в боте → `telegram_id` пишется в `auth.users`

### С сайта (настройки → Telegram-уведомления)
1. Пользователь включает ползунок → `POST /auth/telegram/link/web`
2. Открывается `https://t.me/<bot>?start=wl...`
3. Бот вызывает `POST /telegram/link/web/confirm` → привязка + `notify_telegram=true`

Нужен env на auth: `TELEGRAM_BOT_USERNAME` (username бота без `@`).

## Env (in `infra/docker/.env`)

```env
TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_INTERNAL_TOKEN=<тот же, что NOTIFICATION_SERVICE_TOKEN>
SITE_URL=https://cloud-hustle.com
TELEGRAM_CHANNEL_URL=https://t.me/...
TELEGRAM_SUPPORT_URL=https://cloud-hustle.com/vps/support

# Ops alerts (dedicated fail и т.п.) — те же значения, что в cloud-hustle-monitoring/.env
TELEGRAM_ALERTS_BOT_TOKEN=<токен алерт-бота из monitoring>
TELEGRAM_ALERTS_CHAT_ID=<TELEGRAM_CHAT_ID из monitoring>
```

На сервисе `auth` тоже нужен `TELEGRAM_INTERNAL_TOKEN` (уже в compose).

## Деплой

1. Создать бота у `@BotFather`
2. Прописать `TELEGRAM_BOT_TOKEN` в `.env` на Back
3. Запушить `testVPStrade` → дождаться GHCR image `testvps-trade-telegram-bot`
4. На Back:

```bash
cd /opt/testVPStrade
git pull
bash infra/scripts/deploy-back.sh
```

Либо точечно:

```bash
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml --env-file .env pull telegram-bot auth
docker compose -f docker-compose.back.yml --env-file .env up -d telegram-bot auth
```

Без токена сервис в compose под профилем `telegram` (не стартует по умолчанию).

```bash
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml --env-file .env --profile telegram pull telegram-bot auth
docker compose -f docker-compose.back.yml --env-file .env --profile telegram up -d telegram-bot auth
```

Либо добавьте в `.env`: `COMPOSE_PROFILES=telegram` и обычный `deploy-back.sh`.

