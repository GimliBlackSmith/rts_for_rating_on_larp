# rts_for_rating_on_larp

## Local development stack

This repository ships with a Docker based local environment. It bundles the Go application
with Postgres and Redis for a complete development stack and leverages
[Air](https://github.com/cosmtrek/air) for hot reloading and
[Gore](https://github.com/motemen/gore) for an interactive REPL.

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

- Application container exposes port `8080`.
- Postgres credentials: `rts_user` / `rts_password`, database `rts_db`.
- Redis is persisted via a named volume and exposes port `6379`.

Adjust environment variables or ports in `docker-compose.yml` as needed for your application.
