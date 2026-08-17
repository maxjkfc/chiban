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
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
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

// TestPassword is used by every fixture that does not care about the password.
const TestPassword = "correct-horse-battery"

// User is an account created by a fixture helper.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// RegisterUser creates an account and leaves the App's client logged in as it.
// Registration returns a session, which is what the invite flow relies on.
func (a *App) RegisterUser(email string) User {
	a.t.Helper()

	resp := a.Request(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": TestPassword,
	})
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("register %s: status = %d, want %d", email, resp.StatusCode, http.StatusCreated)
	}

	var user User
	a.DecodeJSON(resp, &user)
	return user
}

// Profile is the profile shape the API returns.
type Profile struct {
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

// SaveProfile completes onboarding for the currently logged-in user.
func (a *App) SaveProfile(displayName, timezone string) Profile {
	a.t.Helper()

	resp := a.Request(http.MethodPatch, "/api/v1/me/profile", map[string]string{
		"display_name": displayName,
		"timezone":     timezone,
	})
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("save profile: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var p Profile
	a.DecodeJSON(resp, &p)
	return p
}

// Group is the group shape the API returns.
type Group struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	IsOwner bool   `json:"is_owner"`
}

// Invite is the invite shape the API returns.
type Invite struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
}

// Onboard registers a user, completes their profile and leaves the client
// logged in as them. Most tests only care that a usable account exists.
func (a *App) Onboard(email, displayName string) User {
	a.t.Helper()

	user := a.RegisterUser(email)
	a.SaveProfile(displayName, "Asia/Taipei")
	return user
}

// CreateGroup creates a group owned by the currently logged-in user.
func (a *App) CreateGroup(name string) Group {
	a.t.Helper()

	resp := a.Request(http.MethodPost, "/api/v1/groups", map[string]string{"name": name})
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("create group: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var g Group
	a.DecodeJSON(resp, &g)
	return g
}

// CreateInvite issues an invite for the group as the current user.
func (a *App) CreateInvite(groupID string) Invite {
	a.t.Helper()

	resp := a.Request(http.MethodPost, "/api/v1/groups/"+groupID+"/invites", nil)
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("create invite: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var i Invite
	a.DecodeJSON(resp, &i)
	return i
}

// JoinGroup redeems an invite code as the current user.
func (a *App) JoinGroup(code string) Group {
	a.t.Helper()

	resp := a.Request(http.MethodPost, "/api/v1/groups/join", map[string]string{"code": code})
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("join group: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var g Group
	a.DecodeJSON(resp, &g)
	return g
}

// Meal is the meal shape the API returns.
type Meal struct {
	ID          string   `json:"id"`
	MealType    string   `json:"meal_type"`
	EatenAt     string   `json:"eaten_at"`
	Description string   `json:"description"`
	PhotoIDs    []string `json:"photo_ids"`
}

// JPEG returns a small valid JPEG, for tests that need a real photo rather
// than bytes that merely claim to be one.
func JPEG(t *testing.T, width, height int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// UploadMeal posts a meal with the given photos as multipart form data, the
// way the browser does.
func (a *App) UploadMeal(fields map[string]string, photos ...[]byte) *http.Response {
	a.t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := form.WriteField(name, value); err != nil {
			a.t.Fatalf("write field %s: %v", name, err)
		}
	}
	for i, photo := range photos {
		part, err := form.CreateFormFile("photos", fmt.Sprintf("photo-%d.jpg", i))
		if err != nil {
			a.t.Fatalf("create photo part: %v", err)
		}
		if _, err := part.Write(photo); err != nil {
			a.t.Fatalf("write photo: %v", err)
		}
	}
	if err := form.Close(); err != nil {
		a.t.Fatalf("close form: %v", err)
	}

	req, err := http.NewRequestWithContext(a.t.Context(), http.MethodPost, a.BaseURL+"/api/v1/meals", &body)
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := a.Client.Do(req)
	if err != nil {
		a.t.Fatalf("upload meal: %v", err)
	}
	a.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// CreateMeal uploads a meal and fails the test unless it was created.
func (a *App) CreateMeal(photos ...[]byte) Meal {
	a.t.Helper()

	resp := a.UploadMeal(nil, photos...)
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("create meal: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var m Meal
	a.DecodeJSON(resp, &m)
	return m
}

// CreateMealAt uploads a meal eaten at a specific instant, for tests about
// which day it lands on.
func (a *App) CreateMealAt(eatenAt time.Time, photos ...[]byte) Meal {
	a.t.Helper()

	resp := a.UploadMeal(map[string]string{
		"eaten_at": eatenAt.UTC().Format(time.RFC3339),
	}, photos...)
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("create meal: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var m Meal
	a.DecodeJSON(resp, &m)
	return m
}

// MealsOn returns the meals the API reports for a calendar date.
func (a *App) MealsOn(date string) []Meal {
	a.t.Helper()

	resp := a.Request(http.MethodGet, "/api/v1/meals?date="+date, nil)
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("meals on %s: status = %d, want %d", date, resp.StatusCode, http.StatusOK)
	}

	var day struct {
		Date  string `json:"date"`
		Meals []Meal `json:"meals"`
	}
	a.DecodeJSON(resp, &day)
	return day.Meals
}

// SessionCookie returns the current session token, for tests that need to
// replay or tamper with it.
func (a *App) SessionCookie() string {
	a.t.Helper()

	base, err := url.Parse(a.BaseURL)
	if err != nil {
		a.t.Fatalf("parse base url: %v", err)
	}
	for _, cookie := range a.Client.Jar.Cookies(base) {
		if cookie.Name == auth.CookieName {
			return cookie.Value
		}
	}
	a.t.Fatal("no session cookie; is the client logged in?")
	return ""
}

// SetSessionCookie forces the client to present the given session token.
func (a *App) SetSessionCookie(token string) {
	a.t.Helper()

	base, err := url.Parse(a.BaseURL)
	if err != nil {
		a.t.Fatalf("parse base url: %v", err)
	}
	a.Client.Jar.SetCookies(base, []*http.Cookie{{
		Name:  auth.CookieName,
		Value: token,
		Path:  "/",
	}})
}

// Logout ends the current session.
func (a *App) Logout() {
	a.t.Helper()

	resp := a.Request(http.MethodPost, "/api/v1/auth/logout", nil)
	if resp.StatusCode != http.StatusNoContent {
		a.t.Fatalf("logout: status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
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
