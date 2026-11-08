.PHONY: build test test-coverage lint run migrate-up migrate-status docker-up docker-down

# Build application binaries
build:
	go build -v -o bin/cms-server ./cmd/server
	go build -v -o bin/cms-migrate ./cmd/migrate

# Run all unit and integration tests
test:
	go test -v -race ./...

# Run test coverage report
test-coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

# Code formatting and static vet
lint:
	go fmt ./...
	go vet ./...

# Run server locally
run:
	go run ./cmd/server

# Apply database migrations
migrate-up:
	go run ./cmd/migrate up

# Check database migration status
migrate-status:
	go run ./cmd/migrate status

# Launch database and infrastructure containers
docker-up:
	docker compose up -d postgres redis

# Stop all containers
docker-down:
	docker compose down

