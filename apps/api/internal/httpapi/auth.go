package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func newUserResponse(u auth.User) userResponse {
	return userResponse{ID: u.ID.String(), Email: u.Email}
}

// registerHandler creates the account and logs the new user straight in, so
// the invite flow does not make someone type their password twice.
func registerHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req credentialsRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		user, err := d.Auth.Register(r.Context(), req.Email, req.Password)
		if err != nil {
			writeAuthError(w, d, err)
			return
		}
		if !startSession(w, r, d, user) {
			return
		}
		writeJSON(w, http.StatusCreated, newUserResponse(user))
	}
}

func loginHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req credentialsRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		user, err := d.Auth.Authenticate(r.Context(), req.Email, req.Password)
		if err != nil {
			writeAuthError(w, d, err)
			return
		}
		if !startSession(w, r, d, user) {
			return
		}
		writeJSON(w, http.StatusOK, newUserResponse(user))
	}
}

func logoutHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.Auth.EndSession(r.Context(), auth.TokenFromRequest(r)); err != nil {
			writeAuthError(w, d, err)
			return
		}
		auth.ClearCookie(w, d.SecureCookies)
		w.WriteHeader(http.StatusNoContent)
	}
}

func meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, newUserResponse(auth.UserFromContext(r.Context())))
	}
}

func startSession(w http.ResponseWriter, r *http.Request, d Deps, user auth.User) bool {
	session, err := d.Auth.StartSession(r.Context(), user.ID)
	if err != nil {
		writeAuthError(w, d, err)
		return false
	}
	auth.SetCookie(w, session, d.SecureCookies)
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
		return false
	}
	return true
}

// writeAuthError maps domain errors to status codes and keeps internal
// failures from reaching the client as detail.
func writeAuthError(w http.ResponseWriter, d Deps, err error) {
	var invalid auth.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message, invalid.Field)
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email is already registered", "email")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid email or password")
	default:
		d.Logger.Error("auth request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
