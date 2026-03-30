# rts_for_rating_on_larp

Telegram-бот для рейтинга игроков LARP с webhook-интеграцией, Postgres, Redis и Nginx/TLS.

## Что поднимается

Базовый стек (`make up`):
- `app` — Go-приложение с Telegram webhook и admin API
- `nginx` — TLS-терминация и прокси на `app`
- `postgres-primary` — основная БД
- `redis` — кэш/служебное хранилище

Опционально через профили:
- `ha`: `postgres-replica`, `pgpool`
- `observability`: `prometheus`, `postgres-exporter`, `grafana`

## Быстрый старт

### 1) Подготовить `.env`

```bash
TELEGRAM_TOKEN=ваш_токен_бота
ADMIN_TOKEN=сложный_токен_для_admin
WEBHOOK_URL=https://bot.example.com
BOT_LINK_BASE=https://t.me/your_bot_username
# WEBHOOK_PATH=/webhook          # опционально (по умолчанию /webhook)
# WEBHOOK_CERT=                  # только для self-signed
```

> `WEBHOOK_URL` указывайте **без** `/webhook` — путь добавляется из `WEBHOOK_PATH`.

### 2) Подложить TLS-сертификат

Nginx читает:
- `nginx/certs/server.crt`
- `nginx/certs/server.key`

Для production положите сюда валидный сертификат вашего домена.

### 3) Запуск

```bash
make build
make up
```

### 4) Проверка

```bash
curl -k https://bot.example.com/healthz
curl "https://api.telegram.org/bot${TELEGRAM_TOKEN}/getWebhookInfo"
```

## Важно: если `/start` не приходит после смены сертификата

Рабочий сценарий восстановления webhook:

```bash
curl "https://api.telegram.org/bot${TELEGRAM_TOKEN}/deleteWebhook?drop_pending_updates=false"
curl -X POST "https://api.telegram.org/bot${TELEGRAM_TOKEN}/setWebhook" \
  -d "url=https://bot.example.com/webhook"
curl "https://api.telegram.org/bot${TELEGRAM_TOKEN}/getWebhookInfo"
```

Дополнительно:
- если у вас **публичный** сертификат (Let's Encrypt, GlobalSign и т.д.), оставляйте `WEBHOOK_CERT` пустым;
- `WEBHOOK_CERT` нужен только для self-signed сертификатов.

### Как стать первым админом

1. Напишите боту `/start`, чтобы создался профиль игрока.
2. Узнайте свой `telegram_id` (например, через `@userinfobot`).
3. Выполните `/create_admin <ваш_telegram_id>`.

Если админов в системе еще нет, вы будете назначены `super_admin`.

## Команды бота

### Пользовательские
- `/start [payload]` — приветствие/инициализация профиля; поддержка deep-link payload.
- `/register [роль] <полное имя>` — создать/обновить анкету персонажа.
- `/my_link` — получить персональную ссылку и QR-код.
- `/transfer <telegram_id> <сумма>` — перевод рейтинга другому игроку.

### Админские
- `/add_player <telegram_id> <полное имя>` — добавить игрока.
- `/set_cycle_duration <минуты>` — длительность цикла, минимум 15 минут.
- `/set_rating_timeout <минуты>` — таймаут на повторную оценку (> 0).
- `/set_rating_limits <уровень 1-5> <лимит>` — лимит оценок за цикл для уровня.
- `/set_level_boundary <уровень 1-5> <мин> <макс>` — границы уровня.
- `/apply_level_recalc` — пересчитать уровни по границам.
- `/create_admin <telegram_id>` — назначить администратора.

## Полезные команды разработки

| Команда | Что делает |
| --- | --- |
| `make build` | Сборка образов |
| `make up` | Запуск базового стека |
| `make up-ha` | Запуск с HA-профилем |
| `make up-observability` | Запуск с мониторингом |
| `make down` | Остановить контейнеры |
| `make clean` | Остановить + удалить тома |
| `make logs` | Логи приложения |
| `make sh` | Shell в `app` |
| `make psql` | Подключиться к Postgres |
| `make redis-cli` | Подключиться к Redis |

## Замечания по эксплуатации

- Для webhook должны быть открыты входящие `443/tcp` (и `80/tcp`, если используете certbot standalone).
- Откройте порты в firewall/security group: `443/tcp` для Telegram webhook и `80/tcp` для certbot/redirect.
- При использовании CDN/прокси (например Cloudflare) для диагностики сначала включайте режим DNS only.
- После обновления сертификата перезапускайте `nginx` (`docker compose restart nginx`).
