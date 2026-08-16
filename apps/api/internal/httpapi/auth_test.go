package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

func TestRegisterThenLoginReturnsTheSameUser(t *testing.T) {
	app := testsupport.NewApp(t)

	registered := app.RegisterUser("mei@example.com")
	app.Logout()

	resp := app.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "mei@example.com",
		"password": testsupport.TestPassword,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var loggedIn testsupport.User
	app.DecodeJSON(resp, &loggedIn)
	if loggedIn.ID != registered.ID {
		t.Fatalf("login returned user %s, want %s", loggedIn.ID, registered.ID)
	}
}

// Registration logs the new user in, so someone arriving through an invite
// link does not have to enter their password a second time.
func TestRegisterStartsASession(t *testing.T) {
	app := testsupport.NewApp(t)

	registered := app.RegisterUser("mei@example.com")

	resp := app.Request(http.MethodGet, "/api/v1/auth/me", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var me testsupport.User
	app.DecodeJSON(resp, &me)
	if me.ID != registered.ID {
		t.Fatalf("me returned %s, want %s", me.ID, registered.ID)
	}
}

func TestMeWithoutSessionIsUnauthorized(t *testing.T) {
	app := testsupport.NewApp(t)

	resp := app.Request(http.MethodGet, "/api/v1/auth/me", nil)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// Logging out has to invalidate the session server-side, not just drop the
// cookie: a copied token must stop working.
func TestLogoutInvalidatesTheSessionServerSide(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")

	token := app.SessionCookie()
	app.Logout()
	app.SetSessionCookie(token)

	resp := app.Request(http.MethodGet, "/api/v1/auth/me", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")

	if _, err := app.DB.ExecContext(t.Context(),
		`UPDATE sessions SET expires_at = now() - interval '1 second'`,
	); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	resp := app.Request(http.MethodGet, "/api/v1/auth/me", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRegisteringATakenEmailConflicts(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")

	resp := app.Request(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":    "mei@example.com",
		"password": testsupport.TestPassword,
	})

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

// Emails differing only in case are the same account, enforced by the citext
// column rather than by callers remembering to lowercase.
func TestEmailUniquenessIgnoresCase(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")

	resp := app.Request(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":    "MEI@Example.com",
		"password": testsupport.TestPassword,
	})

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestLoginWithWrongPasswordIsUnauthorized(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.Logout()

	resp := app.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "mei@example.com",
		"password": "not-the-password",
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// An unknown email and a wrong password must be indistinguishable, otherwise
// the login form becomes an account-enumeration oracle.
func TestLoginDoesNotRevealWhetherAnAccountExists(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.Logout()

	wrongPassword := app.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "mei@example.com",
		"password": "not-the-password",
	})
	var wrongPasswordBody map[string]string
	app.DecodeJSON(wrongPassword, &wrongPasswordBody)

	unknownEmail := app.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "nobody@example.com",
		"password": "not-the-password",
	})
	var unknownEmailBody map[string]string
	app.DecodeJSON(unknownEmail, &unknownEmailBody)

	if wrongPassword.StatusCode != unknownEmail.StatusCode {
		t.Fatalf("status %d for wrong password but %d for unknown email",
			wrongPassword.StatusCode, unknownEmail.StatusCode)
	}
	if wrongPasswordBody["error"] != unknownEmailBody["error"] {
		t.Fatalf("error %q for wrong password but %q for unknown email",
			wrongPasswordBody["error"], unknownEmailBody["error"])
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		password string
	}{
		{"missing email", "", testsupport.TestPassword},
		{"malformed email", "not-an-email", testsupport.TestPassword},
		{"short password", "mei@example.com", "short"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := testsupport.NewApp(t)

			resp := app.Request(http.MethodPost, "/api/v1/auth/register", map[string]string{
				"email":    tc.email,
				"password": tc.password,
			})

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

// A database dump must not hand out passwords or live session tokens.
func TestSecretsAreNotStoredInRecoverableForm(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	sessionToken := app.SessionCookie()

	var passwordHash string
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT password_hash FROM users WHERE email = 'mei@example.com'`,
	).Scan(&passwordHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	if strings.Contains(passwordHash, testsupport.TestPassword) {
		t.Fatal("password is recoverable from the users table")
	}

	var storedTokens int
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sessions WHERE encode(token_hash, 'escape') = $1`, sessionToken,
	).Scan(&storedTokens); err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	if storedTokens != 0 {
		t.Fatal("session token is stored verbatim")
	}
}
