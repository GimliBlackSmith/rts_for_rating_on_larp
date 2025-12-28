# rts_for_rating_on_larp

## Local development stack

This repository ships with a Docker based local environment. It bundles the Go application
with Postgres, Redis, and observability tooling for a complete development stack. It also
includes TLS termination for webhook testing and a streaming replica for Postgres.

### Prerequisites

- Docker 20.10+
- Docker Compose v2 (`docker compose` CLI)

### Useful commands

| Command | Description |
| --- | --- |
| `make build` | Build the development images. |
| `make up` | Start the stack in the background. |
| `make down` | Stop the stack and remove containers. |
| `make clean` | Remove containers and named volumes. |
| `make logs` | Tail the application logs. |
| `make sh` | Open a shell inside the application container. |
| `make psql` | Connect to Postgres using `psql`. |
| `make redis-cli` | Connect to Redis using `redis-cli`. |
| `make gore` | Launch the Go REPL (`gore`) in the application container. |
| `make air` | Run the application in the foreground with Air (useful for one-off sessions). |

### Environment details

- Application container exposes port `8080` (internal) and is proxied via Nginx with TLS on `https://localhost:8443`.
- Postgres primary is available on `localhost:5432`, replica on `localhost:5433`, and Pgpool on `localhost:5434`.
- Postgres credentials: `rts_user` / `rts_password`, database `rts_db`.
- Redis is persisted via a named volume and exposes port `6379`.
- Prometheus is available on `http://localhost:9090`.
- Grafana is available on `http://localhost:3000` (default credentials `admin` / `admin`).
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
     - `WEBHOOK_URL` (default in compose is `https://localhost:8443`)
     - `SERVER_ADDR` (default `:8080`)

2. **Provision TLS certificate**
   - For local testing, the stack generates a self-signed cert in `nginx/certs`.
   - For real deployments, replace `nginx/certs/server.crt` and `nginx/certs/server.key`
     with a valid certificate for your domain.

3. **Build the images**
   ```bash
   make build
   ```

4. **Start the full cluster**
   ```bash
   make up
   ```
   This launches:
   - `postgres-primary`, `postgres-replica`
   - `pgpool` for read/write pooling
   - `nginx` for TLS termination
   - `app` (the bot API + webhook server)
   - `prometheus`, `grafana`, `postgres-exporter`

5. **Verify service health**
   - Webhook endpoint (behind TLS): `https://localhost:8443/healthz`
   - Admin UI: `https://localhost:8443/admin?token=ADMIN_TOKEN`
   - Metrics: `http://localhost:9090`
   - Grafana: `http://localhost:3000` (admin/admin)

6. **Register Telegram webhook**
   - Ensure your public DNS/TLS is set and reachable by Telegram.
   - Set `WEBHOOK_URL=https://your-domain` in `.env`.
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
