APP_NAME := hank-remote

.PHONY: tidy fmt build run-cloud run-agent run-db-ops

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
