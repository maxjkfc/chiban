.PHONY: dev dev-deps down logs test fmt vet migrate-up migrate-down

COMPOSE ?= docker compose

# Tests run against the same PostgreSQL and fake-gcs the app uses.
export CHIBAN_TEST_DATABASE_URL ?= postgres://chiban:chiban@localhost:$(or $(POSTGRES_PORT),15432)/chiban?sslmode=disable
export CHIBAN_TEST_STORAGE_EMULATOR_HOST ?= http://localhost:$(or $(FAKE_GCS_PORT),14443)
export CHIBAN_DATABASE_URL ?= $(CHIBAN_TEST_DATABASE_URL)

## Start the whole stack.
dev:
	$(COMPOSE) up --build

## Start only the backing services, for running api/web from source.
dev-deps:
	$(COMPOSE) up -d postgres fake-gcs

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

## Run the Go suite. -p 1 because the integration tests share one database.
test: dev-deps
	cd apps/api && go test -p 1 ./...

fmt:
	cd apps/api && gofmt -l -w .

vet:
	cd apps/api && go vet ./...

migrate-up:
	cd apps/api && go run ./cmd/migrate up

migrate-down:
	cd apps/api && go run ./cmd/migrate down
