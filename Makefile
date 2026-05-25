.DEFAULT_GOAL := help

SHELL := /bin/bash

.PHONY: help
help: ## List targets
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z_-]+:.*## /{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---------------------------------------------------------------------------
# Local stack
# ---------------------------------------------------------------------------

.PHONY: docker-up
docker-up: ## Start MySQL in docker-compose (detached, wait healthy)
	docker compose up -d mysql

.PHONY: docker-up-all
docker-up-all: ## Start MySQL + API in docker-compose
	docker compose --profile app up -d --build

.PHONY: docker-down
docker-down: ## Stop and remove all compose containers
	docker compose --profile app --profile migrate down

.PHONY: docker-nuke
docker-nuke: ## Stop containers and delete the MySQL volume
	docker compose --profile app --profile migrate down -v

# ---------------------------------------------------------------------------
# Migrations (golang-migrate)
# ---------------------------------------------------------------------------

.PHONY: migrate-up
migrate-up: ## Apply all migrations (uses dockerized migrate)
	docker compose --profile migrate run --rm migrate up

.PHONY: migrate-down
migrate-down: ## Roll back one migration step
	docker compose --profile migrate run --rm migrate down 1

.PHONY: migrate-force
migrate-force: ## Force migration version (VERSION=N)
	docker compose --profile migrate run --rm migrate force $(VERSION)

.PHONY: migrate-new
migrate-new: ## Create a new migration pair (NAME=description)
	docker compose --profile migrate run --rm migrate create -ext sql -dir /migrations -seq $(NAME)

# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------

.PHONY: run
run: ## Run the API locally (reads .env if present)
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi; \
	go run ./cmd/api

.PHONY: build
build: ## Build static binary into ./bin/api
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

# ---------------------------------------------------------------------------
# Tests / lint
# ---------------------------------------------------------------------------

.PHONY: test
test: ## Run unit tests (short)
	go test -count=1 -short ./...

.PHONY: test-race
test-race: ## Run unit tests with -race
	go test -count=1 -race -short ./...

.PHONY: test-integration
test-integration: ## Run integration tests (testcontainers; needs Docker)
	go test -count=1 -tags=integration ./...

.PHONY: test-acceptance
test-acceptance: ## Run the Postman collection with newman (needs the API up)
	newman run api/postman/league-api.postman_collection.json

.PHONY: cover
cover: ## Coverage report into coverage.out + coverage.html
	go test -count=1 -short -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage.html written"

.PHONY: fmt
fmt: ## Format all Go sources in place (gofmt -s -w)
	gofmt -s -w .

.PHONY: lint
lint: ## Check formatting + vet + staticcheck + golangci-lint
	@unformatted=$$(gofmt -s -l .); \
		if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed; skipping"
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed; skipping"

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy
