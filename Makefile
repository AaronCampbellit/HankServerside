APP_NAME := hank-remote
COMPOSE := docker compose --env-file .env.cloud

.PHONY: tidy fmt build frontend-install frontend-test frontend-build frontend-check build-all run-cloud run-agent run-db-ops migrate-up migrate-status migrate-baseline schema-drift-check loadtest

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

build: frontend-build
	go build ./...

frontend-install:
	npm --prefix web/dashboard install

frontend-test:
	npm --prefix web/dashboard run test:run

frontend-build:
	npm --prefix web/dashboard run build

frontend-check:
	npm --prefix web/dashboard run check

build-all: build

run-cloud:
	go run ./cmd/hank-remote-cloud

run-agent:
	go run ./cmd/hank-remote-agent

run-db-ops:
	go run ./cmd/hank-db-ops

migrate-up:
	@if [ -f .env.cloud ] && command -v docker >/dev/null 2>&1 && $(COMPOSE) ps --services --status running 2>/dev/null | grep -qx postgres; then \
		$(COMPOSE) run -T --rm --entrypoint /usr/local/bin/hank-remote-cloud cloud migrate up; \
	else \
		set -a; [ ! -f .env.cloud ] || . ./.env.cloud; set +a; \
		go run ./cmd/hank-remote-cloud migrate up; \
	fi

migrate-status:
	@if [ -f .env.cloud ] && command -v docker >/dev/null 2>&1 && $(COMPOSE) ps --services --status running 2>/dev/null | grep -qx postgres; then \
		$(COMPOSE) run -T --rm --entrypoint /usr/local/bin/hank-remote-cloud cloud migrate status --strict; \
	else \
		set -a; [ ! -f .env.cloud ] || . ./.env.cloud; set +a; \
		go run ./cmd/hank-remote-cloud migrate status --strict; \
	fi

migrate-baseline:
	@if [ -f .env.cloud ] && command -v docker >/dev/null 2>&1 && $(COMPOSE) ps --services --status running 2>/dev/null | grep -qx postgres; then \
		$(COMPOSE) run -T --rm --entrypoint /usr/local/bin/hank-remote-cloud cloud migrate baseline; \
	else \
		set -a; [ ! -f .env.cloud ] || . ./.env.cloud; set +a; \
		go run ./cmd/hank-remote-cloud migrate baseline; \
	fi

schema-drift-check:
	scripts/schema-drift-check.sh

loadtest:
	go test ./tools/loadtest
