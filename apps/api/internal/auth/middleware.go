package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// CookieName is the session cookie. It is HttpOnly, so no frontend code ever
// reads it; the browser just sends it back.
const CookieName = "chiban_session"

type contextKey struct{}

var userContextKey contextKey

// SetCookie writes the session cookie. secure should be false only for plain
// HTTP local development.
func SetCookie(w http.ResponseWriter, session Session, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie expires the session cookie in the browser.
func ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// TokenFromRequest reads the session token, if any.
func TokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// RequireUser rejects requests without a valid session before the wrapped
// handler runs, so no handler has to remember to check.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.UserForToken(r.Context(), TokenFromRequest(r))
		if err != nil {
			if errors.Is(err, ErrNoSession) {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
	})
}

// writeJSONError keeps middleware rejections in the same shape as handler
// errors, so the frontend has one error contract.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// WithUser stores the authenticated user on the context.
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext returns the authenticated user. It panics if called outside
// a handler wrapped in RequireUser: that is a wiring bug, not a runtime
// condition, and failing loudly beats silently treating the request as
// anonymous.
func UserFromContext(ctx context.Context) User {
	user, ok := ctx.Value(userContextKey).(User)
	if !ok {
		panic("auth: no user in context; handler is not wrapped in RequireUser")
	}
	return user
}
