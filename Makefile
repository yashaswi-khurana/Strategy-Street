.PHONY: all up down migrate migrate-up migrate-down migrate-status seed seed-candles tf-verify api engine web test test-go test-engine test-web lint lint-go lint-engine lint-web help

-include .env
export

COMPOSE_FILE ?= deploy/docker-compose.yml
DATABASE_URL ?= postgres://postgres:postgres@localhost:5433/strategystreet?sslmode=disable
REDIS_URL ?= redis://localhost:6379/0

ENV_FILE_FLAG := $(shell [ -f .env ] && echo "--env-file .env")
MIGRATE_BIN := $(shell which migrate 2>/dev/null || ( [ -x $(HOME)/go/bin/migrate ] && echo $(HOME)/go/bin/migrate ))

all: test lint

up:
	docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) up -d --build

down:
	docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) down

migrate: migrate-up

migrate-up:
	@if [ -n "$(MIGRATE_BIN)" ]; then \
		echo "Running migrations up via local migrate CLI..."; \
		$(MIGRATE_BIN) -path db/migrations -database "$(DATABASE_URL)" up; \
	else \
		echo "Running migrations up via Docker Compose..."; \
		docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) run --rm migrate up; \
	fi

migrate-down:
	@if [ -n "$(MIGRATE_BIN)" ]; then \
		echo "Running migrations down via local migrate CLI..."; \
		$(MIGRATE_BIN) -path db/migrations -database "$(DATABASE_URL)" down 1; \
	else \
		echo "Running migrations down via Docker Compose..."; \
		docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) run --rm migrate down 1; \
	fi

migrate-status:
	@if [ -n "$(MIGRATE_BIN)" ]; then \
		echo "Migration version:"; \
		$(MIGRATE_BIN) -path db/migrations -database "$(DATABASE_URL)" version; \
	else \
		echo "Migration version via Docker Compose:"; \
		docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) run --rm migrate version; \
	fi

seed:
	@echo "==> Seeding database..."
	@if command -v psql >/dev/null 2>&1; then \
		for f in $$(ls db/seeds/*.sql | sort); do \
			echo "Applying seed $$f..."; \
			PGPASSWORD="$${POSTGRES_PASSWORD:-postgres}" psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$$f" || exit 1; \
		done; \
	else \
		for f in $$(ls db/seeds/*.sql | sort); do \
			echo "Applying seed $$f via Docker Compose..."; \
			docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) exec -T timescaledb psql -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-strategystreet}" -v ON_ERROR_STOP=1 < "$$f" || exit 1; \
		done; \
	fi

seed-candles:
	@echo "==> Seeding synthetic 1m candle data and refreshing continuous aggregates..."
	cd api && go run ./cmd/seed-candles

tf-verify:
	@echo "==> Verifying continuous aggregates against from-scratch time_bucket computation..."
	cd api && go run ./cmd/tf-verify

api:
	cd api && go run ./cmd/api

engine:
	cd engine && (uv run python -m engine.main 2>/dev/null || .venv/bin/python -m engine.main 2>/dev/null || python3 -m engine.main)

web:
	cd web && npm start

test: test-go test-engine test-web

test-go:
	@echo "==> Running Go unit tests..."
	cd api && go test -v ./...

test-engine:
	@echo "==> Running Python engine unit tests..."
	cd engine && (uv run pytest --cov=engine tests/ 2>/dev/null || .venv/bin/pytest --cov=engine tests/)

test-web:
	@echo "==> Running Angular unit tests..."
	cd web && npm test -- --watch=false

lint: lint-go lint-engine lint-web

lint-go:
	@echo "==> Linting Go code..."
	cd api && go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd api && golangci-lint run ./...; \
	fi

lint-engine:
	@echo "==> Linting Python engine..."
	cd engine && (uv run ruff check . 2>/dev/null || .venv/bin/ruff check .)
	cd engine && (uv run mypy . 2>/dev/null || .venv/bin/mypy .)

lint-web:
	@echo "==> Linting Angular workspace..."
	cd web && npm run lint

help:
	@echo "Usage:"
	@echo "  make up             - Start all containers via Docker Compose"
	@echo "  make down           - Stop all containers"
	@echo "  make migrate        - Run database migrations (alias for migrate-up)"
	@echo "  make migrate-up     - Run all up migrations"
	@echo "  make migrate-down   - Roll back latest migration"
	@echo "  make migrate-status - Show current migration version"
	@echo "  make seed           - Seed database with reference data"
	@echo "  make api            - Run Go API service locally"
	@echo "  make engine         - Run Python Engine service locally"
	@echo "  make web            - Run Angular frontend dev server"
	@echo "  make test           - Run all unit tests (Go, Python, Angular)"
	@echo "  make lint           - Run all linters (Go, Python, Angular)"
