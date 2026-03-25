.PHONY: run migrate test lint

run:
	go run ./cmd/server

migrate:
	go run ./cmd/migrate up

test:
	go test ./...

lint:
	go vet ./...

