.PHONY: dev dev-deps down logs test test-db run-api run-web fmt vet migrate-up migrate-down

# Make does not read .env on its own, so without this every port and credential
# below would silently disagree with what Compose actually uses the moment
# anyone edits .env.
-include .env
export

COMPOSE ?= docker compose
POSTGRES_HOST_PORT ?= $(or $(POSTGRES_PORT),15432)
FAKE_GCS_HOST_PORT ?= $(or $(FAKE_GCS_PORT),14443)
API_HOST_PORT ?= $(or $(API_PORT),18080)
PG_USER ?= $(or $(POSTGRES_USER),chiban)
PG_PASSWORD ?= $(or $(POSTGRES_PASSWORD),chiban)
APP_DB ?= $(or $(POSTGRES_DB),chiban)
TEST_DB ?= $(APP_DB)_test
PG_READY_TIMEOUT ?= 30

export CHIBAN_DATABASE_URL ?= postgres://$(PG_USER):$(PG_PASSWORD)@localhost:$(POSTGRES_HOST_PORT)/$(APP_DB)?sslmode=disable
export CHIBAN_STORAGE_EMULATOR_HOST ?= http://localhost:$(FAKE_GCS_HOST_PORT)
# Source-run API listens on the same port Compose publishes, so the web dev
# server reaches it either way. Stop the api container first.
export CHIBAN_HTTP_ADDR ?= :$(API_HOST_PORT)
export NEXT_PUBLIC_API_BASE_URL ?= http://localhost:$(API_HOST_PORT)

# Tests get their own database. They TRUNCATE every table and roll migrations
# back to zero, which must never happen to the database the app is running on.
export CHIBAN_TEST_DATABASE_URL ?= postgres://$(PG_USER):$(PG_PASSWORD)@localhost:$(POSTGRES_HOST_PORT)/$(TEST_DB)?sslmode=disable
export CHIBAN_TEST_STORAGE_EMULATOR_HOST ?= $(CHIBAN_STORAGE_EMULATOR_HOST)

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

## Create the throwaway test database if it does not exist yet.
test-db: dev-deps
	@i=0; until $(COMPOSE) exec -T postgres pg_isready -U $(PG_USER) -q; do \
		i=$$((i+1)); \
		if [ $$i -ge $(PG_READY_TIMEOUT) ]; then \
			echo "postgres was not ready after $(PG_READY_TIMEOUT)s:" >&2; \
			$(COMPOSE) exec -T postgres pg_isready -U $(PG_USER) >&2 || true; \
			exit 1; \
		fi; \
		sleep 1; \
	done
	@$(COMPOSE) exec -T postgres psql -U $(PG_USER) -d postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname='$(TEST_DB)'" | grep -q 1 \
		|| $(COMPOSE) exec -T postgres createdb -U $(PG_USER) $(TEST_DB)

## Run the Go suite. -p 1 because the integration tests share one database.
test: test-db
	cd apps/api && go test -p 1 ./...

## Run the API from source against the compose backing services.
run-api: dev-deps
	cd apps/api && go run ./cmd/api

run-web:
	cd apps/web && npm run dev

fmt:
	cd apps/api && gofmt -l -w .

vet:
	cd apps/api && go vet ./...

migrate-up:
	cd apps/api && go run ./cmd/migrate up

migrate-down:
	cd apps/api && go run ./cmd/migrate down
