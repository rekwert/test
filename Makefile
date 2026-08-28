.PHONY: up down migrate seed test lint logs

COMPOSE = docker compose -f infra/docker/docker-compose.yml --env-file .env

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

migrate:
	bash infra/scripts/migrate.sh

seed:
	bash infra/scripts/seed.sh

test:
	cd services/auth && go test ./...
	cd services/billing && go test ./...
	cd services/vps && go test ./...
	cd services/notification && go test ./...
	cd apps/gateway && go test ./...

lint:
	cd services/auth && go vet ./...
	cd services/billing && go vet ./...
	cd apps/gateway && go vet ./...

logs:
	$(COMPOSE) logs -f
