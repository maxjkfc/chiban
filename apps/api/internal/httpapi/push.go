package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/push"
)

type subscribePushRequest struct {
	DeviceID string `json:"device_id"`
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	UserAgent string `json:"user_agent,omitempty"`
}

type unsubscribePushRequest struct {
	Endpoint string `json:"endpoint"`
}

func vapidPublicKeyHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubKey := ""
		if d.Push != nil {
			pubKey = d.VAPIDPublicKey
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"public_key": pubKey,
		})
	}
}

func subscribePushHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())

		var req subscribePushRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed json payload")
			return
		}

		if err := push.ValidateEndpoint(req.Endpoint); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_endpoint", err.Error())
			return
		}

		if req.DeviceID == "" || req.Keys.P256DH == "" || req.Keys.Auth == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "device_id, endpoint, and keys are required")
			return
		}

		ua := req.UserAgent
		if ua == "" {
			ua = r.UserAgent()
		}

		sub, err := d.PushStore.Upsert(r.Context(), push.Subscription{
			UserID:    user.ID,
			DeviceID:  req.DeviceID,
			Endpoint:  req.Endpoint,
			P256DH:    req.Keys.P256DH,
			Auth:      req.Keys.Auth,
			UserAgent: ua,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to save subscription")
			return
		}

		writeJSON(w, http.StatusOK, sub)
	}
}

func unsubscribePushHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())

		var req unsubscribePushRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed json payload")
			return
		}

		if req.Endpoint == "" {
			writeError(w, http.StatusBadRequest, "missing_endpoint", "endpoint is required")
			return
		}

		if err := d.PushStore.DeleteByEndpoint(r.Context(), user.ID, req.Endpoint); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to unsubscribe")
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
