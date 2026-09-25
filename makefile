SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
export ALLURE_RESULTS_DIR := $(CURDIR)/allure-results

COMPOSE_DEV := docker compose -f compose.yml -f compose.dev.yml
COMPOSE_PROD := docker compose -f compose.yml
COMPOSE_TEST := docker compose -f compose.test.yml
TEST_DATABASE_URL := postgres://sudostream:sudostream@localhost:5433/sudostream_test?sslmode=disable

TEST_COVERPKG := $(shell go list ./cmd/... ./internal/... | grep -vE 'sudoStream/(openapi|internal/frontend)$$' | paste -sd, -)
GO_PKGS := ./cmd/... ./internal/...

help:
	@echo ""
	@echo "Available targets:"
	@echo "  check             Run linter and formatter in dry-run mode"
	@echo "  fix               Run linter and formatter in fix mode"
	@echo "  test              Run tests with coverage (excludes generated packages)"
	@echo "  test-db-up        Start Postgres for integration tests (port 5433)"
	@echo "  test-db-down      Stop Postgres for integration tests (port 5433)"
	@echo "  openapi           Generate openapi/ (OpenAPI 3.1 JSON + YAML)"
	@echo "  frontend-install  Install web dependencies (pnpm)"
	@echo "  frontend-typegen  Generate type schemas from OpenAPI"
	@echo "  frontend-check    Run OXC linter"
	@echo "  frontend-fix      Run OXC formatter"
	@echo "  frontend-dev      Run Vite dev server"
	@echo "  frontend-build    Build web/dist for Go binary embed"
	@echo "  docker-build      Build prod image (distroless cc-debian13:nonroot)"
	@echo "  dev-up            Start dev stack (debug-nonroot, shell via busybox)"
	@echo "  dev-down          Stop dev stack"
	@echo "  prod-up           Start prod stack (GHCR image)"
	@echo "  prod-down         Stop prod stack"
	@echo "  docs              Generate and serve docs locally"
	@echo ""

check:
	-golangci-lint fmt --diff-colored $(GO_PKGS)
	-golangci-lint run $(GO_PKGS)
	-hadolint Dockerfile

fix:
	golangci-lint fmt $(GO_PKGS)
	golangci-lint run --fix $(GO_PKGS)

test-db-up:
	$(COMPOSE_TEST) up -d --wait

test-db-down:
	$(COMPOSE_TEST) down -v

test: test-db-up
	rm -rf "$(ALLURE_RESULTS_DIR)"
	mkdir -p "$(ALLURE_RESULTS_DIR)"
	SUDOSTREAM_DATABASE_URL=$(TEST_DATABASE_URL) GIN_MODE=test go test -race -covermode=atomic -coverprofile=coverage.txt -coverpkg=$(TEST_COVERPKG) -count=1 $(GO_PKGS)
	go tool cover -func=coverage.txt
	go tool cover -html=coverage.txt -o coverage.html

openapi:
	go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc6 init \
	  --v3.1 --parseDependency --parseInternal \
	  --outputTypes json,yaml \
	  --output ./openapi -g cmd/server/main.go

frontend-install:
	cd web && pnpm install --frozen-lockfile

frontend-typegen:
	cd web && pnpm typegen

frontend-check:
	cd web && pnpm lint && pnpm fmt:check

frontend-fix:
	cd web && pnpm lint:fix && pnpm fmt

frontend-dev:
	cd web && pnpm dev

frontend-build:
	cd web && pnpm run build
	rm -rf internal/frontend/dist
	cp -r web/dist internal/frontend/dist

docker-build:
	DOCKER_BUILDKIT=1 docker build --build-arg DISTROLESS_TAG=nonroot -t sudostream:local .

dev-up:
	$(COMPOSE_DEV) up -d --build

dev-down:
	$(COMPOSE_DEV) down

prod-up:
	$(COMPOSE_PROD) up -d --build

prod-down:
	$(COMPOSE_PROD) down

docs:
	uv run mkdocs serve

.PHONY: help check fix test test-db-up test-db-down openapi frontend-install frontend-typegen \
	frontend-check frontend-fix frontend-dev frontend-build docker-build dev-up dev-down prod-up prod-down docs
