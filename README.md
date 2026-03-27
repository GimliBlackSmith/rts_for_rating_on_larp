# rts_for_rating_on_larp

## Local development stack

This repository ships with a Docker based environment. By default it runs a minimal set
of services (Go app + Postgres + Redis + Nginx). HA and observability components are now
optional via Compose profiles.

### Prerequisites

- Docker 20.10+
- Docker Compose v2 (`docker compose` CLI)

### Useful commands

| Command | Description |
| --- | --- |
| `make build` | Build images. |
| `make up` | Start the stack in the background. |
| `make up-ha` | Start stack + HA services (`postgres-replica`, `pgpool`). |
| `make up-observability` | Start stack + Prometheus/Grafana/exporter. |
| `make down` | Stop the stack and remove containers. |
| `make clean` | Remove containers and named volumes. |
| `make logs` | Tail the application logs. |
| `make sh` | Open a shell inside the application container. |
| `make psql` | Connect to Postgres using `psql`. |
| `make redis-cli` | Connect to Redis using `redis-cli`. |

### Environment details

- Application container exposes port `8080` (internal) and is proxied via Nginx with TLS on `https://localhost:8443`.
- Postgres primary is available on `localhost:5432`.
- Replica (`localhost:5433`) and Pgpool (`localhost:5434`) are available only with `ha` profile.
- Postgres credentials: `rts_user` / `rts_password`, database `rts_db`.
- Redis is persisted via a named volume and exposes port `6379`.
- Prometheus and Grafana are available only with `observability` profile.
- Application environment expects `TELEGRAM_TOKEN` to be provided (for example via `.env`).
- Admin UI expects `ADMIN_TOKEN` to be provided for access.

### TLS for webhook testing

The stack generates a dummy self-signed certificate on startup in `nginx/certs`. Replace it
with a real certificate before exposing the stack publicly.

### Configuration storage

System configuration values (rating formula constants, cycle duration, timeouts, rating limits)
should be stored in Postgres rather than hard-coded in memory. This keeps multiple bot
instances consistent when using Pgpool in front of the cluster. A sample schema lives at
`db/schema/system_config.sql`.

### Observability

Prometheus scrapes the application `/metrics` endpoint and Postgres metrics via the exporter.
Grafana is available for dashboards.

### Alpha bot commands

- `/register [роль] <полное имя>` — зарегистрировать/обновить анкету персонажа.
- `/my_link` — получить личную ссылку и QR-код.
- `/transfer <telegram_id> <сумма>` — перевести рейтинг.
- `/set_level_boundary <уровень 1-5> <мин> <макс>` — задать границы уровней для цикла.
- `/apply_level_recalc` — применить пересчет уровней по заданным границам.

### Cluster setup & deployment (Docker Compose)

1. **Prepare environment variables**
   - Create a `.env` file at the repository root:
     ```bash
     TELEGRAM_TOKEN=your_bot_token
     ADMIN_TOKEN=your_admin_token
     ```
   - Optional overrides (only if you need to customize):
     - `WEBHOOK_URL` (empty by default; set this to your public HTTPS domain for Telegram webhook registration)
     - `SERVER_ADDR` (default `:8080`)

2. **Provision TLS certificate**
   - For local testing, the stack generates a self-signed cert in `nginx/certs`.
   - For real deployments, replace `nginx/certs/server.crt` and `nginx/certs/server.key`
     with a valid certificate for your domain.

3. **Build the images**
   ```bash
   make build
   ```

4. **Start the base cluster**
   ```bash
   make up
   ```
   This launches:
   - `postgres-primary`
   - `nginx` for TLS termination
   - `app` (the bot API + webhook server)

   Optional:
   - HA profile: `make up-ha`
   - Observability profile: `make up-observability`

5. **Verify service health**
   - Webhook endpoint (behind TLS): `https://localhost:8443/healthz`
   - Admin UI: `https://localhost:8443/admin?token=ADMIN_TOKEN`
   - Metrics/Grafana endpoints are available when running with `observability` profile.

6. **Register Telegram webhook**
   - Ensure your public DNS/TLS is set and reachable by Telegram.
   - Set `WEBHOOK_URL=https://your-domain` in `.env` (**without** `/webhook`; path is controlled by `WEBHOOK_PATH`, default `/webhook`).
   - Restart the app container:
     ```bash
     docker compose restart app
     ```

7. **Scaling the bot**
   - You can run multiple app instances:
     ```bash
     docker compose up -d --scale app=2
     ```
   - All instances share settings through Postgres, keeping configuration consistent.

Adjust environment variables or ports in `docker-compose.yml` as needed for your application.

## Быстрый деплой на VPS + домен (production)

Ниже — минимальный рабочий сценарий для Ubuntu 22.04/24.04.

### Если у вас уже есть домен и готовый сертификат

Используйте этот короткий путь:

1. Скопируйте сертификат и ключ в проект:
   ```bash
   cp /path/to/your/fullchain.pem nginx/certs/server.crt
   cp /path/to/your/privkey.pem nginx/certs/server.key
   chmod 600 nginx/certs/server.key
   ```
2. В `.env` задайте:
   ```bash
   TELEGRAM_TOKEN=ваш_токен_бота
   ADMIN_TOKEN=сложный_секрет_для_admin
   WEBHOOK_URL=https://bot.example.com  # без /webhook
   ```
3. Для production поменяйте в `docker-compose.yml` у `nginx`:
   - было: `8443:443`
   - стало: `443:443`
4. Запустите сервис:
   ```bash
   make build
   make up
   ```
5. Проверьте:
   ```bash
   curl -k https://bot.example.com/healthz
   curl "https://api.telegram.org/bot${TELEGRAM_TOKEN}/getWebhookInfo"
   ```
   В `getWebhookInfo` должен быть ваш домен.

1. **Подготовьте DNS**
   - Определите, какое имя будет принимать webhook, например `bot.example.com`.
   - У регистратора домена или в DNS-провайдере (Cloudflare/Route53/Reg.ru и т.д.) создайте запись:
     - Тип: `A`
     - Имя/Host: `bot` (если нужен `bot.example.com`)
     - Значение/IPv4: `<ПУБЛИЧНЫЙ_IP_ВАШЕГО_VPS>`
     - TTL: `300` секунд (или `Auto`).
   - Если у вас есть IPv6 на VPS, добавьте `AAAA` запись:
     - Тип: `AAAA`
     - Имя/Host: `bot`
     - Значение/IPv6: `<IPv6_ВАШЕГО_VPS>`
   - Если используете Cloudflare и Telegram webhook, на первом запуске лучше поставить режим **DNS only** (серая тучка), чтобы исключить проблемы с проксированием TLS.
   - Проверьте, что DNS уже резолвится в нужный IP:
     ```bash
     dig +short bot.example.com A
     dig +short bot.example.com AAAA
     nslookup bot.example.com
     ```
   - Убедитесь, что возвращается именно IP вашего сервера, и только после этого переходите к выпуску сертификата Let's Encrypt.

2. **Установите Docker и Compose plugin**
   ```bash
   sudo apt update
   sudo apt install -y ca-certificates curl gnupg
   sudo install -m 0755 -d /etc/apt/keyrings
   curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
   echo \
     "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
     $(. /etc/os-release && echo $VERSION_CODENAME) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
   sudo apt update
   sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
   sudo usermod -aG docker "$USER"
   ```
   После этого перелогиньтесь в SSH-сессию.

3. **Склонируйте сервис и заполните `.env`**
   ```bash
   git clone <URL_ВАШЕГО_РЕПОЗИТОРИЯ>
   cd rts_for_rating_on_larp
   cat > .env <<'EOF'
   TELEGRAM_TOKEN=ваш_токен_бота
   ADMIN_TOKEN=сложный_секрет_для_admin
   WEBHOOK_URL=https://bot.example.com  # без /webhook
   EOF
   ```

4. **Получите TLS-сертификат Let's Encrypt**
   В текущем `docker-compose.yml` nginx читает сертификаты из:
   - `nginx/certs/server.crt`
   - `nginx/certs/server.key`

   Проще всего выпустить сертификат certbot и скопировать файлы в эти пути:
   ```bash
   sudo apt install -y certbot
   sudo certbot certonly --standalone -d bot.example.com

   sudo cp /etc/letsencrypt/live/bot.example.com/fullchain.pem nginx/certs/server.crt
   sudo cp /etc/letsencrypt/live/bot.example.com/privkey.pem nginx/certs/server.key
   sudo chown $USER:$USER nginx/certs/server.crt nginx/certs/server.key
   chmod 600 nginx/certs/server.key
   ```

5. **Откройте порты в firewall/security group**
   - `22/tcp` (SSH)
   - `80/tcp` (для certbot renew hook/редиректов)
   - `443/tcp` (входящий webhook от Telegram)

   В текущем compose внешний TLS-порт проброшен как `8443:443`.
   Для production лучше заменить на `443:443` в `docker-compose.yml`.

6. **Запустите стек**
   ```bash
   make build
   make up
   ```

7. **Проверьте, что сервис поднялся**
   ```bash
   docker compose ps
   curl -k https://bot.example.com/healthz
   ```
   Если вернулся `ok`/`200`, переходите дальше.

8. **Проверьте установку webhook у Telegram**
   ```bash
   curl "https://api.telegram.org/bot${TELEGRAM_TOKEN}/getWebhookInfo"
   ```
   В ответе должен быть URL вида `https://bot.example.com/...`.

9. **Добавьте автообновление сертификата**
   После renew нужно перезагружать nginx:
   ```bash
   cat <<'EOF' | sudo tee /etc/cron.d/certbot-rts
   SHELL=/bin/bash
   PATH=/usr/sbin:/usr/bin:/sbin:/bin
   17 3 * * * root certbot renew --quiet && cd /path/to/rts_for_rating_on_larp && cp /etc/letsencrypt/live/bot.example.com/fullchain.pem nginx/certs/server.crt && cp /etc/letsencrypt/live/bot.example.com/privkey.pem nginx/certs/server.key && docker compose restart nginx
   EOF
   ```

### Что важно учесть для продакшена

- Air удален из запуска сервиса: контейнер `app` использует production runtime-образ и запускает бинарник напрямую.
- По умолчанию выключены HA/observability сервисы (через profiles), чтобы снизить потребление RAM/CPU.
- Для `app`, `postgres-primary`, `redis`, `nginx` заданы лимиты CPU/RAM в `docker-compose.yml`.
- Порты Postgres/Redis/Grafana/Prometheus стоит ограничить firewall-правилами или убрать наружную публикацию.
- Обязательно храните `.env` и резервные копии базы отдельно от кода.
