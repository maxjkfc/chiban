// Package testsupport boots the real API against a real PostgreSQL database
// for integration tests.
//
// V0.1 has one primary test seam: tests drive the HTTP API and the WebSocket
// endpoint, exactly as a browser would. Authorization is the highest-risk part
// of this product and it is only real once middleware, handlers and services
// are wired together, so tests never call services directly.
//
// Object storage is the one substituted dependency: tests use the in-memory
// ObjectStorage implementation, which also lets them simulate upload failures.
package testsupport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/database"
	"github.com/maxjkfc/chiban/apps/api/internal/httpapi"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

const databaseURLEnv = "CHIBAN_TEST_DATABASE_URL"

var (
	migrateOnce sync.Once
	migrateErr  error
)

// App is a running API instance plus the handles a test needs to inspect it.
type App struct {
	t       *testing.T
	BaseURL string
	DB      *sql.DB
	Storage *storage.Memory
	// Client keeps cookies, so a test that logs in stays logged in.
	Client *http.Client
}

// NewApp starts an API server for a single test and cleans up afterwards.
// Each call starts from an empty database.
func NewApp(t *testing.T) *App {
	t.Helper()

	databaseURL := os.Getenv(databaseURLEnv)
	if databaseURL == "" {
		t.Fatalf("%s is not set. Start the local stack with `make dev-deps` and run tests with `make test`.", databaseURLEnv)
	}

	ctx := t.Context()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	migrateOnce.Do(func() { migrateErr = database.Up(ctx, db) })
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	truncateAll(t, ctx, db)

	objects := storage.NewMemory()
	router := httpapi.NewRouter(httpapi.Deps{
		DB:      db,
		Storage: objects,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}

	return &App{
		t:       t,
		BaseURL: server.URL,
		DB:      db,
		Storage: objects,
		Client:  &http.Client{Jar: jar},
	}
}

// Request sends a JSON request to the API. A nil body sends no payload.
func (a *App) Request(method, path string, body any) *http.Response {
	a.t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("encode request body: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(a.t.Context(), method, a.BaseURL+path, payload)
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	a.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// DecodeJSON reads a JSON response body into dest.
func (a *App) DecodeJSON(resp *http.Response, dest any) {
	a.t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		a.t.Fatalf("decode response body: %v", err)
	}
}

// truncateAll empties every application table so tests start from a known
// state.
//
// ponytail: TRUNCATE is fast enough while the schema is small and the suite
// runs serially; move to a template database if the suite gets slow.
func truncateAll(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'goose_db_version'
	`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, `"`+table+`"`)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) == 0 {
		return
	}

	stmt := "TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}
