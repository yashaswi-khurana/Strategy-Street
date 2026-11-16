.PHONY: all up down migrate api engine web test lint help

-include .env
export

COMPOSE_FILE ?= deploy/docker-compose.yml
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/strategystreet?sslmode=disable
REDIS_URL ?= redis://localhost:6379/0

ENV_FILE_FLAG := $(shell [ -f .env ] && echo "--env-file .env")

all: test lint

up:
	docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) up -d --build

down:
	docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) down

migrate:
	@if command -v migrate >/dev/null 2>&1; then \
		echo "Running migrations via local migrate CLI..."; \
		migrate -path db/migrations -database "$(DATABASE_URL)" up; \
	else \
		echo "Running migrations via Docker Compose..."; \
		docker compose -f $(COMPOSE_FILE) $(ENV_FILE_FLAG) run --rm migrate up; \
	fi

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
	@echo "  make up       - Start all containers via Docker Compose"
	@echo "  make down     - Stop all containers"
	@echo "  make migrate  - Run database migrations"
	@echo "  make api      - Run Go API service locally"
	@echo "  make engine   - Run Python Engine service locally"
	@echo "  make web      - Run Angular frontend dev server"
	@echo "  make test     - Run all unit tests (Go, Python, Angular)"
	@echo "  make lint     - Run all linters (Go, Python, Angular)"
