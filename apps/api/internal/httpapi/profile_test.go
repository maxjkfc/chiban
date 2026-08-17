package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// A missing profile is how the frontend knows to send someone to onboarding,
// so it has to be distinguishable from an error.
func TestProfileIsAbsentUntilOnboardingCompletes(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")

	resp := app.Request(http.MethodGet, "/api/v1/me/profile", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestOnboardingCreatesTheProfile(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")

	created := app.Request(http.MethodPatch, "/api/v1/me/profile", map[string]string{
		"display_name": "小美",
		"timezone":     "Asia/Taipei",
	})
	if created.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want %d", created.StatusCode, http.StatusOK)
	}

	resp := app.Request(http.MethodGet, "/api/v1/me/profile", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var p testsupport.Profile
	app.DecodeJSON(resp, &p)
	if p.DisplayName != "小美" || p.Timezone != "Asia/Taipei" {
		t.Fatalf("profile = %+v, want 小美 / Asia/Taipei", p)
	}
}

// PATCH has to leave untouched what the client did not send, otherwise editing
// a nickname would silently reset the timezone.
func TestPatchLeavesOmittedFieldsAlone(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	resp := app.Request(http.MethodPatch, "/api/v1/me/profile", map[string]string{
		"display_name": "美美",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var p testsupport.Profile
	app.DecodeJSON(resp, &p)
	if p.DisplayName != "美美" {
		t.Fatalf("display name = %q, want 美美", p.DisplayName)
	}
	if p.Timezone != "Asia/Taipei" {
		t.Fatalf("timezone = %q, want it left at Asia/Taipei", p.Timezone)
	}
}

// Timezone decides where a user's day starts, so an unloadable zone must never
// reach the database — every Today and history query would break on it later.
func TestProfileRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		body map[string]string
	}{
		{"missing display name", map[string]string{"timezone": "Asia/Taipei"}},
		{"blank display name", map[string]string{"display_name": "   ", "timezone": "Asia/Taipei"}},
		{"over-long display name", map[string]string{"display_name": strings.Repeat("あ", 51), "timezone": "Asia/Taipei"}},
		{"missing timezone", map[string]string{"display_name": "小美"}},
		{"made-up timezone", map[string]string{"display_name": "小美", "timezone": "Mars/Olympus"}},
		{"offset instead of zone", map[string]string{"display_name": "小美", "timezone": "+08:00"}},
		{"server-local timezone", map[string]string{"display_name": "小美", "timezone": "Local"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := testsupport.NewApp(t)
			app.RegisterUser("mei@example.com")

			resp := app.Request(http.MethodPatch, "/api/v1/me/profile", tc.body)

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

// Real IANA zones must load inside the API's own binary. The deployment image
// carries no system tzdata, so this fails there unless time/tzdata is embedded.
func TestProfileAcceptsRealTimezones(t *testing.T) {
	for _, zone := range []string{"Asia/Taipei", "Asia/Tokyo", "America/New_York", "UTC"} {
		t.Run(zone, func(t *testing.T) {
			app := testsupport.NewApp(t)
			app.RegisterUser("mei@example.com")

			resp := app.Request(http.MethodPatch, "/api/v1/me/profile", map[string]string{
				"display_name": "小美",
				"timezone":     zone,
			})

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestProfileRequiresASession(t *testing.T) {
	app := testsupport.NewApp(t)

	get := app.Request(http.MethodGet, "/api/v1/me/profile", nil)
	if get.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET status = %d, want %d", get.StatusCode, http.StatusUnauthorized)
	}

	patch := app.Request(http.MethodPatch, "/api/v1/me/profile", map[string]string{
		"display_name": "小美",
		"timezone":     "Asia/Taipei",
	})
	if patch.StatusCode != http.StatusUnauthorized {
		t.Fatalf("PATCH status = %d, want %d", patch.StatusCode, http.StatusUnauthorized)
	}
}

// One user's profile edit must not be able to touch another's row.
func TestProfileIsScopedToTheSessionUser(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")
	app.Logout()

	app.RegisterUser("kai@example.com")
	app.SaveProfile("阿凱", "Asia/Tokyo")

	var meiName, meiZone string
	if err := app.DB.QueryRowContext(t.Context(), `
		SELECT p.display_name, p.timezone
		FROM profiles p JOIN users u ON u.id = p.user_id
		WHERE u.email = 'mei@example.com'
	`).Scan(&meiName, &meiZone); err != nil {
		t.Fatalf("read mei's profile: %v", err)
	}
	if meiName != "小美" || meiZone != "Asia/Taipei" {
		t.Fatalf("mei's profile became %q / %q", meiName, meiZone)
	}
}
