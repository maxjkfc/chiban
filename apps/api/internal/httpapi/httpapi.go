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
	"github.com/maxjkfc/chiban/apps/api/internal/chat"
	"github.com/maxjkfc/chiban/apps/api/internal/group"
	"github.com/maxjkfc/chiban/apps/api/internal/meal"
	"github.com/maxjkfc/chiban/apps/api/internal/profile"
	"github.com/maxjkfc/chiban/apps/api/internal/push"
	"github.com/maxjkfc/chiban/apps/api/internal/sticker"
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
	Chat    *chat.Service
	Sticker *sticker.Service
	// Hub fans realtime messages out to connected members. One per process:
	// V0.1 is a single node, so a broadcast only has to reach this process.
	Hub *chat.Hub
	// WebOrigin is the one browser origin allowed to send credentialed
	// requests. Empty disables CORS entirely, which is what tests want.
	WebOrigin string
	// SecureCookies marks session cookies Secure. False only for plain HTTP
	// local development.
	SecureCookies   bool
	Push            *push.Service
	PushStore       *push.Store
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
	FocusManager    *push.FocusManager
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
	if d.Hub == nil {
		d.Hub = chat.NewHub()
	}
	if d.Sticker == nil {
		// A sticker is a personal picture, so who may look at one is the same
		// question an avatar asks: are these two in a group together.
		d.Sticker = sticker.NewService(d.DB, d.Storage, d.Group)
	}
	if d.Chat == nil {
		// Group membership is what scopes who may read or write in a chat, and
		// the sticker domain says whether a sticker is the sender's to send.
		d.Chat = chat.NewService(d.DB, d.Group, d.Hub, d.Storage, d.Sticker)
	}
	if d.Meal == nil {
		// Sharing widens who may read a meal, so meal asks the group domain
		// which groups a reader is in, and asks chat to announce the card.
		d.Meal = meal.NewService(d.DB, d.Storage, d.Group, d.Chat)
	}
	if d.FocusManager == nil {
		d.FocusManager = push.NewFocusManager()
	}
	if d.PushStore == nil && d.DB != nil {
		d.PushStore = push.NewStore(d.DB)
	}
	if d.Push == nil && d.PushStore != nil {
		d.Push = push.NewService(d.PushStore, push.VAPIDKeys{
			PublicKey:  d.VAPIDPublicKey,
			PrivateKey: d.VAPIDPrivateKey,
			Subject:    d.VAPIDSubject,
		}, d.FocusManager)
	}
	if d.Chat != nil && d.Push != nil {
		d.Chat.SetPushNotifier(d.Push)
	}
	if d.Meal != nil && d.Push != nil {
		d.Meal.SetPushNotifier(d.Push)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(d))

	mux.HandleFunc("POST /api/v1/auth/register", registerHandler(d))
	mux.HandleFunc("POST /api/v1/auth/login", loginHandler(d))
	mux.HandleFunc("POST /api/v1/auth/logout", logoutHandler(d))
	mux.Handle("GET /api/v1/auth/me", d.Auth.RequireUser(meHandler()))
	mux.HandleFunc("GET /api/v1/push/vapid-public-key", vapidPublicKeyHandler(d))
	mux.Handle("POST /api/v1/push/subscribe", d.Auth.RequireUser(subscribePushHandler(d)))
	mux.Handle("POST /api/v1/push/unsubscribe", d.Auth.RequireUser(unsubscribePushHandler(d)))
	mux.Handle("GET /api/v1/ws/me", d.Auth.RequireUser(userEventsSocketHandler(d)))

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
	mux.Handle("POST /api/v1/meals/{meal_id}/shares", d.Auth.RequireUser(shareMealHandler(d)))
	mux.Handle("GET /api/v1/meals/{meal_id}/shares", d.Auth.RequireUser(listMealSharesHandler(d)))
	mux.Handle("DELETE /api/v1/meals/{meal_id}/shares/{group_id}", d.Auth.RequireUser(unshareMealHandler(d)))

	mux.Handle("POST /api/v1/groups/{group_id}/messages", d.Auth.RequireUser(sendMessageHandler(d)))
	mux.Handle("GET /api/v1/groups/{group_id}/messages", d.Auth.RequireUser(listMessagesHandler(d)))
	mux.Handle("GET /api/v1/ws/groups/{group_id}", d.Auth.RequireUser(chatSocketHandler(d)))
	mux.Handle("DELETE /api/v1/messages/{message_id}", d.Auth.RequireUser(deleteMessageHandler(d)))
	mux.Handle("POST /api/v1/messages/{message_id}/reactions", d.Auth.RequireUser(addReactionHandler(d)))
	mux.Handle("DELETE /api/v1/messages/{message_id}/reactions/{reaction_type}",
		d.Auth.RequireUser(removeReactionHandler(d)))
	mux.Handle("GET /api/v1/reaction-types", d.Auth.RequireUser(availableReactionsHandler()))
	mux.Handle("POST /api/v1/chat-media", d.Auth.RequireUser(uploadChatMediaHandler(d)))
	mux.Handle("GET /api/v1/chat-media/{media_id}", d.Auth.RequireUser(getChatMediaHandler(d)))

	mux.Handle("POST /api/v1/me/stickers", d.Auth.RequireUser(addStickerHandler(d)))
	mux.Handle("GET /api/v1/me/stickers", d.Auth.RequireUser(listStickersHandler(d)))
	mux.Handle("DELETE /api/v1/me/stickers/{sticker_id}", d.Auth.RequireUser(deleteStickerHandler(d)))
	mux.Handle("PUT /api/v1/me/sticker-pins", d.Auth.RequireUser(setStickerPinsHandler(d)))
	mux.Handle("GET /api/v1/stickers/{sticker_id}/media", d.Auth.RequireUser(getStickerMediaHandler(d)))

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
			// Every method the router registers has to be listed here. A route
			// whose method is missing is refused by the browser at the preflight,
			// which the API never sees and so can never log.
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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

// logFailedUpload records what the client claimed about files that never made
// it into storage.
//
// A phone's camera roll is the one input this product cannot reproduce
// locally, and every rejection reaches the user as the same "it failed". A
// HEIC, a ProRAW DNG and a photo iCloud never finished downloading are three
// different bugs; the declared name, type and size tell them apart without a
// round of guessing. The bytes themselves are never logged.
func logFailedUpload(r *http.Request, d Deps, field string, err error) {
	for _, header := range r.MultipartForm.File[field] {
		d.Logger.WarnContext(r.Context(), "upload not stored",
			"field", field,
			"filename", header.Filename,
			"declared_type", header.Header.Get("Content-Type"),
			"bytes", header.Size,
			"reason", err)
	}
}
