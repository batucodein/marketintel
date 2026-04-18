.PHONY: build run test test-short lint migrate-up migrate-down docker docker-down clean

BINARY=marketintel
GO=go

build:
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o bin/$(BINARY) ./cmd/server

run:
	$(GO) run ./cmd/server

test:
	$(GO) test ./... -v -race -count=1

test-short:
	$(GO) test ./... -short -count=1

lint:
	golangci-lint run ./...

migrate-up:
	migrate -path migrations -database "postgres://marketintel:marketintel@localhost:5433/marketintel?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://marketintel:marketintel@localhost:5433/marketintel?sslmode=disable" down

docker:
	docker compose -f ../docker-compose.yml up --build

docker-down:
	docker compose -f ../docker-compose.yml down

clean:
	rm -rf bin/
