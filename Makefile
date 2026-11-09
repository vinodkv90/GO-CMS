.PHONY: all build build-admin test test-coverage lint run migrate-up migrate-status docker-up docker-down backup restore clean

all: build-admin build test

# Build application backend binaries
build:
	go build -v -o bin/cms-server ./cmd/server
	go build -v -o bin/cms-migrate ./cmd/migrate

# Build React Admin SPA distribution
build-admin:
	cd admin && npm run build

# Run all automated tests across all modules
test:
	go test -v ./...

# Run test coverage report
test-coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

# Code formatting and static analysis
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
	docker compose up -d postgres redis minio

# Stop all containers
docker-down:
	docker compose down

# Create full backup of database and media assets
backup:
	@bash ./scripts/backup.sh ./backups

# Restore from backup archive (specify ARCHIVE=...)
restore:
	@bash ./scripts/restore.sh $(ARCHIVE)

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html admin/dist
