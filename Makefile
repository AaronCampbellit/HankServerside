APP_NAME := hank-remote

.PHONY: tidy fmt build run-cloud run-agent run-db-ops migrate-up migrate-status migrate-baseline schema-drift-check loadtest

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

build:
	go build ./...

run-cloud:
	go run ./cmd/hank-remote-cloud

run-agent:
	go run ./cmd/hank-remote-agent

run-db-ops:
	go run ./cmd/hank-db-ops

migrate-up:
	go run ./cmd/hank-remote-cloud migrate up

migrate-status:
	go run ./cmd/hank-remote-cloud migrate status --strict

migrate-baseline:
	go run ./cmd/hank-remote-cloud migrate baseline

schema-drift-check:
	scripts/schema-drift-check.sh

loadtest:
	go test ./tools/loadtest
