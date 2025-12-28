SHELL := /bin/bash
COMPOSE := docker compose

.PHONY: help build up down restart logs ps sh gore air psql redis-cli clean

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?##' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the development images
	$(COMPOSE) build

up: ## Start the full local stack in the background
	$(COMPOSE) up -d

restart: down up ## Restart the stack

logs: ## Tail logs from the application container
	$(COMPOSE) logs -f app

ps: ## Show service status
	$(COMPOSE) ps

sh: ## Open a shell in the application container
	$(COMPOSE) exec app /bin/bash

psql: ## Open a psql session inside the Postgres primary container
	$(COMPOSE) exec postgres-primary psql -U rts_user -d rts_db

redis-cli: ## Open a redis-cli session
	$(COMPOSE) exec redis redis-cli

gore: ## Launch gore REPL inside the application container
	$(COMPOSE) run --rm app gore

air: ## Run the application with Air in the foreground
	$(COMPOSE) run --rm --service-ports app air -c .air.toml

down: ## Stop and remove containers, networks, and anonymous volumes
	$(COMPOSE) down --remove-orphans

clean: ## Remove all containers and named volumes
	$(COMPOSE) down --volumes --remove-orphans
