# Auralis developer tasks. `make help` lists everything.
SHELL := /bin/bash
COMPOSE ?= docker compose
VENV ?= .venv
PY ?= $(VENV)/bin/python

GO_MODULES := libs/go-platform services/gateway services/auth services/user services/content services/playback services/analytics
PY_SERVICES := services/ai_media services/recommendation

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# --- local stack ---------------------------------------------------------

.PHONY: up
up: ## Build and start the whole stack (infra + 8 services + workers + web)
	@test -f .env || cp .env.example .env
	$(COMPOSE) up -d --build

.PHONY: down
down: ## Stop the stack, keep volumes
	$(COMPOSE) down

.PHONY: clean
clean: ## Stop the stack and delete all data volumes
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Tail logs for all services
	$(COMPOSE) logs -f --tail=100

.PHONY: ps
ps: ## Show container status
	$(COMPOSE) ps

.PHONY: migrate
migrate: ## Run database migrations for every service
	$(COMPOSE) run --rm auth /auth migrate
	$(COMPOSE) run --rm user /user migrate
	$(COMPOSE) run --rm content /content migrate
	$(COMPOSE) run --rm playback /playback migrate
	$(COMPOSE) run --rm analytics /analytics migrate
	$(COMPOSE) run --rm ai-media migrate
	$(COMPOSE) run --rm recommendation migrate

.PHONY: seed
seed: ## Load the fictional catalog and synthetic activity
	SERVICE_SHARED_TOKEN=$$(grep SERVICE_SHARED_TOKEN .env | cut -d= -f2) \
	AUTH_BOOTSTRAP_ADMIN_EMAIL=$$(grep '^ADMIN_EMAIL' .env | cut -d= -f2) \
	AUTH_BOOTSTRAP_ADMIN_PASSWORD=$$(grep '^ADMIN_PASSWORD' .env | cut -d= -f2) \
	$(PY) scripts/seed.py

.PHONY: e2e
e2e: ## Run the critical end-to-end flows against the running stack
	$(PY) scripts/e2e.py

.PHONY: reco-eval
reco-eval: ## Run the offline recommendation evaluation
	$(COMPOSE) run --rm recommendation evaluate --k 10

.PHONY: load
load: ## Run the k6 load test suite (needs k6 installed)
	k6 run load/catalog.js && k6 run load/playback.js

# --- code quality ------------------------------------------------------

.PHONY: fmt
fmt: ## Format Go and Python
	@for m in $(GO_MODULES); do (cd $$m && gofmt -w .); done
	@for s in $(PY_SERVICES) libs/py-common; do $(VENV)/bin/ruff format $$s >/dev/null; done

.PHONY: lint
lint: ## Lint everything
	@for m in $(GO_MODULES); do echo "vet $$m"; (cd $$m && go vet ./...); done
	@for s in $(PY_SERVICES) libs/py-common; do echo "ruff $$s"; $(VENV)/bin/ruff check $$s; done
	@cd frontend && npm run lint

.PHONY: typecheck
typecheck: ## Type-check Python and the frontend
	@for s in $(PY_SERVICES); do $(VENV)/bin/mypy $$s || true; done
	@cd frontend && npm run typecheck

.PHONY: test
test: test-go test-py test-web ## Run every test suite

.PHONY: test-go
test-go: ## Go unit + integration tests (needs local Postgres/Kafka or the compose stack)
	@for m in $(GO_MODULES); do echo "test $$m"; (cd $$m && go test ./...); done

.PHONY: test-py
test-py: ## Python tests
	@for s in $(PY_SERVICES); do echo "test $$s"; (cd $$s && ../../$(PY) -m pytest -q); done

.PHONY: test-web
test-web: ## Frontend tests
	@cd frontend && npm run test

# --- dev environment -------------------------------------------------

.PHONY: dev-setup
dev-setup: ## Create the Python venv and install every package for local testing
	python3 -m venv $(VENV)
	$(VENV)/bin/pip install -q --upgrade pip
	$(VENV)/bin/pip install -q -e libs/py-common -e "services/ai_media[dev]" -e "services/recommendation[dev]"
	cd frontend && npm install
	@echo "Also install local Postgres, Redis, Kafka (or just use 'make up') to run the Go integration tests."
