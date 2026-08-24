package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/maxjkfc/chiban/apps/api/internal/push"
	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

func TestPushEndpoints(t *testing.T) {
	app := testsupport.NewApp(t)

	// 1. GET VAPID public key
	t.Run("get vapid public key", func(t *testing.T) {
		res := app.Request(http.MethodGet, "/api/v1/push/vapid-public-key", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}
		var data map[string]string
		app.DecodeJSON(res, &data)
	})

	// 2. Unauthenticated access should fail
	t.Run("unauthenticated subscribe returns 401", func(t *testing.T) {
		body := map[string]any{
			"device_id": "dev-1",
			"endpoint":  "https://fcm.googleapis.com/fcm/send/1",
		}
		res := app.Request(http.MethodPost, "/api/v1/push/subscribe", body)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", res.StatusCode)
		}
	})

	// Register test user
	_ = app.RegisterUser("pushuser@example.com")

	// 3. Reject invalid / SSRF endpoint
	t.Run("reject ssrf endpoint", func(t *testing.T) {
		payload := map[string]any{
			"device_id": "dev-1",
			"endpoint":  "https://attacker.com/evil/push",
			"keys": map[string]string{
				"p256dh": "key1",
				"auth":   "auth1",
			},
		}
		res := app.Request(http.MethodPost, "/api/v1/push/subscribe", payload)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for ssrf endpoint, got %d", res.StatusCode)
		}
	})

	// 4. Successful subscribe and rotation
	t.Run("successful subscribe and rotation", func(t *testing.T) {
		payload := map[string]any{
			"device_id": "dev-1",
			"endpoint":  "https://fcm.googleapis.com/fcm/send/token-dev-1",
			"keys": map[string]string{
				"p256dh": "key_p256",
				"auth":   "key_auth",
			},
			"user_agent": "Mozilla/5.0 Test",
		}
		res := app.Request(http.MethodPost, "/api/v1/push/subscribe", payload)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on subscribe, got %d", res.StatusCode)
		}

		// Rotate endpoint for same device
		payload["endpoint"] = "https://fcm.googleapis.com/fcm/send/token-dev-1-rotated"
		res = app.Request(http.MethodPost, "/api/v1/push/subscribe", payload)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on rotation, got %d", res.StatusCode)
		}
	})

	// 5. Unsubscribe
	t.Run("unsubscribe by endpoint", func(t *testing.T) {
		payload := map[string]string{
			"endpoint": "https://fcm.googleapis.com/fcm/send/token-dev-1-rotated",
		}
		res := app.Request(http.MethodPost, "/api/v1/push/unsubscribe", payload)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on unsubscribe, got %d", res.StatusCode)
		}
	})

	// 6. Logout with device_id unregisters push
	t.Run("logout with device_id clears push subscription", func(t *testing.T) {
		// Re-subscribe device
		subPayload := map[string]any{
			"device_id": "dev-logout-test",
			"endpoint":  "https://fcm.googleapis.com/fcm/send/token-logout-test",
			"keys": map[string]string{
				"p256dh": "key_p256",
				"auth":   "key_auth",
			},
		}
		res := app.Request(http.MethodPost, "/api/v1/push/subscribe", subPayload)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("subscribe before logout failed: %d", res.StatusCode)
		}

		// Logout passing device_id
		logoutBody := map[string]string{
			"device_id": "dev-logout-test",
		}
		res = app.Request(http.MethodPost, "/api/v1/auth/logout", logoutBody)
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204 on logout, got %d", res.StatusCode)
		}

		// Check subscription is deleted
		pushStore := push.NewStore(app.DB)
		subs, err := pushStore.ListByGroupEligible(t.Context(), uuid.New(), uuid.Nil)
		if err != nil {
			t.Fatalf("list subs: %v", err)
		}
		for _, s := range subs {
			if s.DeviceID == "dev-logout-test" {
				t.Fatalf("subscription for dev-logout-test still exists after logout")
			}
		}
	})
}
