package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/media"
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
	// The client fetches the picture by this ID; the bucket and object name
	// stay in the backend. Empty means no avatar has been uploaded.
	AvatarMediaID string `json:"avatar_media_id,omitempty"`
}

func newProfileResponse(p profile.Profile) profileResponse {
	resp := profileResponse{DisplayName: p.DisplayName, Timezone: p.Timezone}
	if p.HasAvatar() {
		resp.AvatarMediaID = p.AvatarMediaID.String()
	}
	return resp
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

// postAvatarHandler takes one image and makes it the user's picture.
//
// Replacing is a new object with a new media ID rather than an overwrite, so a
// URL the browser already cached can never resolve to a different face.
func postAvatarHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, media.MaxUploadBytes+(1<<20))

		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeError(w, http.StatusBadRequest,
				"request must be multipart/form-data within the size limit")
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("avatar")
		if err != nil {
			writeError(w, http.StatusBadRequest, "an avatar image is required", "avatar")
			return
		}
		defer file.Close()

		if header.Size > media.MaxUploadBytes {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("the image must be smaller than %d MB", media.MaxUploadBytes>>20), "avatar")
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, media.MaxUploadBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the uploaded image", "avatar")
			return
		}

		p, err := d.Profile.SaveAvatar(r.Context(), auth.UserFromContext(r.Context()).ID, data)
		if err != nil {
			writeProfileError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newProfileResponse(p))
	}
}

// getAvatarHandler streams an avatar to any signed-in user who can name it.
// Media IDs are unguessable and only handed out through a group's member list.
func getAvatarHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID, ok := pathUUID(w, r, "media_id")
		if !ok {
			return
		}

		reader, contentType, err := d.Profile.OpenAvatar(r.Context(), mediaID)
		if err != nil {
			writeProfileError(w, d, err)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", contentType)
		// A media ID never points at different bytes, so this is safe to cache
		// hard; a new picture is simply a new URL.
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if _, err := io.Copy(w, reader); err != nil {
			d.Logger.Error("streaming avatar failed", "error", err)
		}
	}
}

func writeProfileError(w http.ResponseWriter, d Deps, err error) {
	var invalid profile.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message, invalid.Field)
	case errors.Is(err, profile.ErrNotFound):
		writeError(w, http.StatusNotFound, "profile not created yet")
	case errors.Is(err, profile.ErrAvatarNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		d.Logger.Error("profile request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
