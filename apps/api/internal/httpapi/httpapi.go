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

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/group"
	"github.com/maxjkfc/chiban/apps/api/internal/meal"
	"github.com/maxjkfc/chiban/apps/api/internal/profile"
	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

type Deps struct {
	DB      *sql.DB
	Storage storage.ObjectStorage
	Logger  *slog.Logger
	Auth    *auth.Service
	Profile *profile.Service
	Group   *group.Service
	Meal    *meal.Service
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
	if d.Group == nil {
		d.Group = group.NewService(d.DB)
	}
	if d.Profile == nil {
		// Group membership is what scopes who may see a picture.
		d.Profile = profile.NewService(d.DB, d.Storage, d.Group)
	}
	if d.Meal == nil {
		d.Meal = meal.NewService(d.DB, d.Storage)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(d))

	mux.HandleFunc("POST /api/v1/auth/register", registerHandler(d))
	mux.HandleFunc("POST /api/v1/auth/login", loginHandler(d))
	mux.HandleFunc("POST /api/v1/auth/logout", logoutHandler(d))
	mux.Handle("GET /api/v1/auth/me", d.Auth.RequireUser(meHandler()))

	mux.Handle("GET /api/v1/me/profile", d.Auth.RequireUser(getProfileHandler(d)))
	mux.Handle("PATCH /api/v1/me/profile", d.Auth.RequireUser(patchProfileHandler(d)))
	mux.Handle("POST /api/v1/me/avatar", d.Auth.RequireUser(postAvatarHandler(d)))
	mux.Handle("GET /api/v1/avatars/{media_id}", d.Auth.RequireUser(getAvatarHandler(d)))

	mux.Handle("POST /api/v1/groups", d.Auth.RequireUser(createGroupHandler(d)))
	mux.Handle("GET /api/v1/groups", d.Auth.RequireUser(listGroupsHandler(d)))
	mux.Handle("POST /api/v1/groups/join", d.Auth.RequireUser(joinGroupHandler(d)))
	mux.Handle("GET /api/v1/groups/{group_id}", d.Auth.RequireUser(getGroupHandler(d)))
	mux.Handle("GET /api/v1/groups/{group_id}/members", d.Auth.RequireUser(listMembersHandler(d)))
	mux.Handle("DELETE /api/v1/groups/{group_id}/members/me", d.Auth.RequireUser(leaveGroupHandler(d)))
	mux.Handle("POST /api/v1/groups/{group_id}/invites", d.Auth.RequireUser(createInviteHandler(d)))
	mux.Handle("GET /api/v1/groups/{group_id}/invites", d.Auth.RequireUser(listInvitesHandler(d)))
	mux.Handle("DELETE /api/v1/groups/{group_id}/invites/{invite_id}", d.Auth.RequireUser(revokeInviteHandler(d)))

	mux.Handle("POST /api/v1/meals", d.Auth.RequireUser(createMealHandler(d)))
	mux.Handle("GET /api/v1/meals", d.Auth.RequireUser(listMealsHandler(d)))
	mux.Handle("GET /api/v1/meals/{meal_id}", d.Auth.RequireUser(getMealHandler(d)))
	mux.Handle("PATCH /api/v1/meals/{meal_id}", d.Auth.RequireUser(patchMealHandler(d)))
	mux.Handle("DELETE /api/v1/meals/{meal_id}", d.Auth.RequireUser(deleteMealHandler(d)))
	mux.Handle("GET /api/v1/meal-images/{image_id}", d.Auth.RequireUser(getMealImageHandler(d)))

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

// timeFormat is the one timestamp format the API emits: RFC 3339 in UTC.
const timeFormat = time.RFC3339

// pathUUID parses a path parameter, answering 404 for anything that is not a
// UUID so that malformed IDs are indistinguishable from unknown ones.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return uuid.UUID{}, false
	}
	return id, true
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
