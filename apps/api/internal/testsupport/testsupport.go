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
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
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

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

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
	DisplayName   string `json:"display_name"`
	Timezone      string `json:"timezone"`
	AvatarMediaID string `json:"avatar_media_id"`
}

// GetProfile reads the current user's profile.
func (a *App) GetProfile() Profile {
	a.t.Helper()

	resp := a.Request(http.MethodGet, "/api/v1/me/profile", nil)
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("get profile: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var p Profile
	a.DecodeJSON(resp, &p)
	return p
}

// PostAvatar uploads an avatar and returns the raw response, for tests that
// expect it to be refused.
func (a *App) PostAvatar(image []byte) *http.Response {
	a.t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("avatar", "avatar.jpg")
	if err != nil {
		a.t.Fatalf("create avatar part: %v", err)
	}
	if _, err := part.Write(image); err != nil {
		a.t.Fatalf("write avatar: %v", err)
	}
	if err := form.Close(); err != nil {
		a.t.Fatalf("close form: %v", err)
	}

	req, err := http.NewRequestWithContext(a.t.Context(), http.MethodPost, a.BaseURL+"/api/v1/me/avatar", &body)
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := a.Client.Do(req)
	if err != nil {
		a.t.Fatalf("upload avatar: %v", err)
	}
	a.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// UploadAvatar uploads an avatar and fails the test unless it was accepted.
func (a *App) UploadAvatar(image []byte) Profile {
	a.t.Helper()

	resp := a.PostAvatar(image)
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("upload avatar: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var p Profile
	a.DecodeJSON(resp, &p)
	return p
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
	ID           string   `json:"id"`
	MealType     string   `json:"meal_type"`
	EatenAt      string   `json:"eaten_at"`
	EatenAtLocal string   `json:"eaten_at_local"`
	Description  string   `json:"description"`
	PhotoIDs     []string `json:"photo_ids"`
	IsOwner      bool     `json:"is_owner"`
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

// Message is the chat message shape the API returns, over both HTTP and the
// socket. One shape for both is the point: a message read from history and the
// same message pushed live must be indistinguishable.
type Message struct {
	ID              string     `json:"id"`
	GroupID         string     `json:"group_id"`
	UserID          string     `json:"user_id"`
	Type            string     `json:"type"`
	Content         string     `json:"content"`
	ClientMessageID string     `json:"client_message_id"`
	MealRecordID    string     `json:"meal_record_id"`
	ChatMediaID     string     `json:"chat_media_id"`
	StickerID       string     `json:"sticker_id"`
	CreatedAt       string     `json:"created_at"`
	Deleted         bool       `json:"deleted"`
	ReplyTo         *ReplyTo   `json:"reply_to"`
	Reactions       []Reaction `json:"reactions"`
}

// ReplyTo is the quoted parent of a reply.
type ReplyTo struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Deleted bool   `json:"deleted"`
}

// Reaction is one emoji's tally on a message.
type Reaction struct {
	Type  string `json:"reaction_type"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

// SocketEvent is one push from the realtime feed.
type SocketEvent struct {
	Type     string   `json:"type"`
	Message  *Message `json:"message"`
	Reaction *struct {
		MessageID string `json:"message_id"`
		UserID    string `json:"user_id"`
		Type      string `json:"reaction_type"`
		Added     bool   `json:"added"`
	} `json:"reaction"`
	MessageID string `json:"message_id"`
}

// Find returns the tally for one emoji, and whether there is one at all.
func (m Message) Find(reactionType string) (Reaction, bool) {
	for _, r := range m.Reactions {
		if r.Type == reactionType {
			return r, true
		}
	}
	return Reaction{}, false
}

// MessagePage is one screen of history.
type MessagePage struct {
	Messages []Message `json:"messages"`
	Before   string    `json:"before"`
}

// PostMessage sends a message with a caller-chosen client id, for tests about
// retries and validation.
func (a *App) PostMessage(groupID, content, clientMessageID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodPost, "/api/v1/groups/"+groupID+"/messages", map[string]string{
		"content":           content,
		"client_message_id": clientMessageID,
	})
}

// SendMessage posts a message the way a fresh send does, with a new client id,
// and fails the test unless it was accepted.
func (a *App) SendMessage(groupID, content string) Message {
	a.t.Helper()

	resp := a.PostMessage(groupID, content, uuid.NewString())
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("send message: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var m Message
	a.DecodeJSON(resp, &m)
	return m
}

// History reads a page of a group's messages. query is appended as-is, so
// tests can pass "?limit=2&before=...".
func (a *App) History(groupID, query string) MessagePage {
	a.t.Helper()

	resp := a.Request(http.MethodGet, "/api/v1/groups/"+groupID+"/messages"+query, nil)
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("message history: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var page MessagePage
	a.DecodeJSON(resp, &page)
	return page
}

// ChatSocket is one open connection to a group's realtime feed.
type ChatSocket struct {
	t    *testing.T
	conn *websocket.Conn
}

// DialChat attempts the WebSocket handshake and returns its HTTP response, so
// a test can assert that a connection was refused and with what status.
func (a *App) DialChat(groupID string) (*ChatSocket, *http.Response) {
	a.t.Helper()

	socketURL := "ws" + strings.TrimPrefix(a.BaseURL, "http") + "/api/v1/ws/groups/" + groupID

	// The cookie jar only serves http(s) URLs, so the session travels as an
	// explicit header here. The server still sees an ordinary cookie.
	header := http.Header{}
	header.Set("Cookie", auth.CookieName+"="+a.SessionCookie())

	conn, resp, err := websocket.Dial(a.t.Context(), socketURL, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		if resp == nil {
			a.t.Fatalf("dial chat socket: %v", err)
		}
		return nil, resp
	}
	a.t.Cleanup(func() { conn.CloseNow() })
	return &ChatSocket{t: a.t, conn: conn}, resp
}

// ConnectChat opens the socket and fails the test unless the handshake worked.
func (a *App) ConnectChat(groupID string) *ChatSocket {
	a.t.Helper()

	socket, resp := a.DialChat(groupID)
	if socket == nil {
		a.t.Fatalf("connect chat socket: status = %d", resp.StatusCode)
	}
	return socket
}

// NextEvent waits for the next push of any kind.
func (s *ChatSocket) NextEvent() SocketEvent {
	s.t.Helper()

	ctx, cancel := context.WithTimeout(s.t.Context(), 5*time.Second)
	defer cancel()

	var event SocketEvent
	if err := wsjson.Read(ctx, s.conn, &event); err != nil {
		s.t.Fatalf("read from chat socket: %v", err)
	}
	return event
}

// Next waits for the next pushed message, failing if something else arrives.
func (s *ChatSocket) Next() Message {
	s.t.Helper()

	event := s.NextEvent()
	if event.Type != "message" || event.Message == nil {
		s.t.Fatalf("expected a message, got a %q event", event.Type)
	}
	return *event.Message
}

// ExpectSilence fails if anything arrives in the next moment. Used where the
// absence of a broadcast is the behaviour under test, such as a retry that
// must not show up on everyone's screen a second time.
func (s *ChatSocket) ExpectSilence() {
	s.t.Helper()

	ctx, cancel := context.WithTimeout(s.t.Context(), 300*time.Millisecond)
	defer cancel()

	var event SocketEvent
	err := wsjson.Read(ctx, s.conn, &event)
	if err == nil {
		s.t.Fatalf("expected no broadcast, got a %q event", event.Type)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		s.t.Fatalf("read from chat socket: %v", err)
	}
}

// ExpectNoMessage fails if a message arrives in the next moment. Unlike
// ExpectSilence it tolerates the connection being closed, which is what the
// server does when it decides this reader is no longer entitled to the feed.
func (s *ChatSocket) ExpectNoMessage() {
	s.t.Helper()

	ctx, cancel := context.WithTimeout(s.t.Context(), 500*time.Millisecond)
	defer cancel()

	var event SocketEvent
	if err := wsjson.Read(ctx, s.conn, &event); err == nil {
		s.t.Fatalf("expected no message, got a %q event", event.Type)
	}
}

// PostReply sends a message that answers another one.
func (a *App) PostReply(groupID, content, replyToMessageID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodPost, "/api/v1/groups/"+groupID+"/messages", map[string]string{
		"content":             content,
		"client_message_id":   uuid.NewString(),
		"reply_to_message_id": replyToMessageID,
	})
}

// Reply sends a reply and fails the test unless it was accepted.
func (a *App) Reply(groupID, content, replyToMessageID string) Message {
	a.t.Helper()

	resp := a.PostReply(groupID, content, replyToMessageID)
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("reply: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var m Message
	a.DecodeJSON(resp, &m)
	return m
}

// PostReaction adds a reaction, returning the raw response for tests that
// expect it to be refused.
func (a *App) PostReaction(messageID, reactionType string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodPost, "/api/v1/messages/"+messageID+"/reactions",
		map[string]string{"reaction_type": reactionType})
}

// React adds a reaction and fails the test unless it was accepted.
func (a *App) React(messageID, reactionType string) {
	a.t.Helper()

	resp := a.PostReaction(messageID, reactionType)
	if resp.StatusCode != http.StatusNoContent {
		a.t.Fatalf("react: status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// Unreact removes one of the caller's reactions.
func (a *App) Unreact(messageID, reactionType string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodDelete,
		"/api/v1/messages/"+messageID+"/reactions/"+url.PathEscape(reactionType), nil)
}

// DeleteMessage tombstones a message.
func (a *App) DeleteMessage(messageID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodDelete, "/api/v1/messages/"+messageID, nil)
}

// ShareMeal offers a meal to groups, returning the raw response for tests that
// expect it to be refused.
func (a *App) ShareMeal(mealID string, groupIDs ...string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodPost, "/api/v1/meals/"+mealID+"/shares",
		map[string][]string{"group_ids": groupIDs})
}

// UnshareMeal takes a meal back from one group.
func (a *App) UnshareMeal(mealID, groupID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodDelete, "/api/v1/meals/"+mealID+"/shares/"+groupID, nil)
}

// ReadMeal fetches a meal as the current user.
func (a *App) ReadMeal(mealID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodGet, "/api/v1/meals/"+mealID, nil)
}

// ReadMealImage fetches a meal photo as the current user.
func (a *App) ReadMealImage(imageID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodGet, "/api/v1/meal-images/"+imageID, nil)
}

// CanSeeMeal reports whether the current user can read a meal and its first
// photo. Both answers come together on purpose: a reader who can open one but
// not the other is the failure this checks for.
func (a *App) CanSeeMeal(meal Meal) (mealOK, photoOK bool) {
	a.t.Helper()

	return a.ReadMeal(meal.ID).StatusCode == http.StatusOK,
		a.ReadMealImage(meal.PhotoIDs[0]).StatusCode == http.StatusOK
}

// ChatMedia is an upload the chat API accepted.
type ChatMedia struct {
	ID   string `json:"id"`
	Type string `json:"media_type"`
}

// AnimatedGIF returns a small multi-frame GIF, for tests that need animation
// to survive rather than bytes that merely claim to be a GIF.
func AnimatedGIF(t *testing.T, width, height, frames int) []byte {
	t.Helper()

	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{}
	for i := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		frame.SetColorIndex(i%width, 0, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animation); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

// PostChatMedia uploads a file, returning the raw response for tests that
// expect it to be refused.
func (a *App) PostChatMedia(data []byte, filename string) *http.Response {
	a.t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		a.t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		a.t.Fatalf("write file: %v", err)
	}
	if err := form.Close(); err != nil {
		a.t.Fatalf("close form: %v", err)
	}

	req, err := http.NewRequestWithContext(a.t.Context(), http.MethodPost,
		a.BaseURL+"/api/v1/chat-media", &body)
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := a.Client.Do(req)
	if err != nil {
		a.t.Fatalf("upload chat media: %v", err)
	}
	a.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// UploadChatMedia uploads a file and fails the test unless it was accepted.
func (a *App) UploadChatMedia(data []byte, filename string) ChatMedia {
	a.t.Helper()

	resp := a.PostChatMedia(data, filename)
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("upload chat media: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var m ChatMedia
	a.DecodeJSON(resp, &m)
	return m
}

// SendMedia posts an uploaded image or GIF into a group.
func (a *App) SendMedia(groupID, mediaID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodPost, "/api/v1/groups/"+groupID+"/messages", map[string]string{
		"client_message_id": uuid.NewString(),
		"chat_media_id":     mediaID,
	})
}

// ReadChatMedia fetches the stored bytes as the current user.
func (a *App) ReadChatMedia(mediaID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodGet, "/api/v1/chat-media/"+mediaID, nil)
}

// Sticker is one entry in a user's library, as the API returns it.
type Sticker struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// PostSticker uploads one sticker and returns the raw response, so tests can
// assert on rejections as well as successes.
func (a *App) PostSticker(data []byte, filename string) *http.Response {
	a.t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		a.t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		a.t.Fatalf("write file: %v", err)
	}
	if err := form.Close(); err != nil {
		a.t.Fatalf("close form: %v", err)
	}

	req, err := http.NewRequestWithContext(a.t.Context(), http.MethodPost,
		a.BaseURL+"/api/v1/me/stickers", &body)
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := a.Client.Do(req)
	if err != nil {
		a.t.Fatalf("upload sticker: %v", err)
	}
	a.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// AddSticker uploads a sticker and fails the test unless it was accepted.
func (a *App) AddSticker(data []byte, filename string) Sticker {
	a.t.Helper()

	resp := a.PostSticker(data, filename)
	if resp.StatusCode != http.StatusCreated {
		a.t.Fatalf("add sticker: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var one Sticker
	a.DecodeJSON(resp, &one)
	return one
}

// ListStickers returns the current user's library.
func (a *App) ListStickers() []Sticker {
	a.t.Helper()

	resp := a.Request(http.MethodGet, "/api/v1/me/stickers", nil)
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("list stickers: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var stickers []Sticker
	a.DecodeJSON(resp, &stickers)
	return stickers
}

// DeleteSticker removes one from the current user's library.
func (a *App) DeleteSticker(stickerID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodDelete, "/api/v1/me/stickers/"+stickerID, nil)
}

// SendSticker posts one of the caller's stickers into a group.
func (a *App) SendSticker(groupID, stickerID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodPost, "/api/v1/groups/"+groupID+"/messages", map[string]string{
		"client_message_id": uuid.NewString(),
		"sticker_id":        stickerID,
	})
}

// ReadSticker fetches a sticker's bytes as the current user.
func (a *App) ReadSticker(stickerID string) *http.Response {
	a.t.Helper()

	return a.Request(http.MethodGet, "/api/v1/stickers/"+stickerID+"/media", nil)
}
