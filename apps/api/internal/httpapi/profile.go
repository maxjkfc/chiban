package httpapi

import (
	"errors"
	"net/http"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/profile"
)

type profileRequest struct {
	// Pointers so an absent field is distinguishable from an empty one:
	// PATCH must leave what the client did not send alone.
	DisplayName *string `json:"display_name"`
	Timezone    *string `json:"timezone"`
}

type profileResponse struct {
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

func newProfileResponse(p profile.Profile) profileResponse {
	return profileResponse{DisplayName: p.DisplayName, Timezone: p.Timezone}
}

// getProfileHandler returns 404 until onboarding has been completed, which is
// how the frontend decides whether to show the onboarding flow.
func getProfileHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := d.Profile.Get(r.Context(), auth.UserFromContext(r.Context()).ID)
		if err != nil {
			writeProfileError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newProfileResponse(p))
	}
}

// patchProfileHandler creates the profile on first call and updates it after,
// so onboarding and later edits go through one code path.
func patchProfileHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req profileRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		p, err := d.Profile.Save(r.Context(), auth.UserFromContext(r.Context()).ID, profile.Input{
			DisplayName: req.DisplayName,
			Timezone:    req.Timezone,
		})
		if err != nil {
			writeProfileError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newProfileResponse(p))
	}
}

func writeProfileError(w http.ResponseWriter, d Deps, err error) {
	var invalid profile.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message, invalid.Field)
	case errors.Is(err, profile.ErrNotFound):
		writeError(w, http.StatusNotFound, "profile not created yet")
	default:
		d.Logger.Error("profile request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
