// Package httpapi wires HTTP routes to application services.
//
// Handlers stay thin: they decode input, call a service and encode the result.
// Domain logic and authorization live in the domain packages.
package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/profile"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

type Deps struct {
	DB      *sql.DB
	Storage storage.ObjectStorage
	Logger  *slog.Logger
	Auth    *auth.Service
	Profile *profile.Service
	// WebOrigin is the one browser origin allowed to send credentialed
	// requests. Empty disables CORS entirely, which is what tests want.
	WebOrigin string
	// SecureCookies marks session cookies Secure. False only for plain HTTP
	// local development.
	SecureCookies bool
}

func NewRouter(d Deps) http.Handler {
	if d.Auth == nil {
		d.Auth = auth.NewService(d.DB)
	}
	if d.Profile == nil {
		d.Profile = profile.NewService(d.DB)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(d))

	mux.HandleFunc("POST /api/v1/auth/register", registerHandler(d))
	mux.HandleFunc("POST /api/v1/auth/login", loginHandler(d))
	mux.HandleFunc("POST /api/v1/auth/logout", logoutHandler(d))
	mux.Handle("GET /api/v1/auth/me", d.Auth.RequireUser(meHandler()))

	mux.Handle("GET /api/v1/me/profile", d.Auth.RequireUser(getProfileHandler(d)))
	mux.Handle("PATCH /api/v1/me/profile", d.Auth.RequireUser(patchProfileHandler(d)))

	return withCORS(d.WebOrigin, mux)
}

// withCORS allows exactly one origin. Sessions are cookie-based, so
// Access-Control-Allow-Origin can never be "*" once auth lands.
func withCORS(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// healthHandler reports whether the API can serve requests, which for V0.1
// means the database is reachable. Storage is not probed: a media outage
// should not take the whole API out of rotation.
func healthHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := d.DB.PingContext(ctx); err != nil {
			d.Logger.ErrorContext(ctx, "health check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError returns the one error shape the frontend has to handle. field is
// optional and names the input the client should correct.
func writeError(w http.ResponseWriter, status int, message string, field ...string) {
	body := map[string]string{"error": message}
	if len(field) > 0 {
		body["field"] = field[0]
	}
	writeJSON(w, status, body)
}
