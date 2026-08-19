package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/media"
	"github.com/maxjkfc/chiban/apps/api/internal/sticker"
)

type stickerResponse struct {
	ID string `json:"id"`
	// Type is "image" or "gif". The client needs it only to know that a GIF
	// will animate; both are rendered the same way.
	Type string `json:"type"`
	// PinOrder is the slot on the owner's quick rail, 1 to sticker.MaxPins.
	// Omitted when the sticker is not pinned, so "not on the rail" is the
	// absence of a slot rather than a zero the client has to interpret.
	PinOrder int `json:"pin_order,omitempty"`
}

type setStickerPinsRequest struct {
	// Ordered: the first id is the leftmost slot on the rail.
	StickerIDs []string `json:"sticker_ids"`
}

func stickerResponses(stickers []sticker.Sticker) []stickerResponse {
	out := make([]stickerResponse, 0, len(stickers))
	for _, one := range stickers {
		out = append(out, stickerResponse{
			ID: one.ID.String(), Type: one.Type, PinOrder: one.PinOrder,
		})
	}
	return out
}

func addStickerHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, media.MaxUploadBytes+(1<<20))

		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeError(w, http.StatusBadRequest,
				"request must be multipart/form-data within the size limit")
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "a file is required", "file")
			return
		}
		defer file.Close()

		if header.Size > media.MaxUploadBytes {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("the file must be smaller than %d MB", media.MaxUploadBytes>>20), "file")
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, media.MaxUploadBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the uploaded file", "file")
			return
		}

		added, err := d.Sticker.Add(r.Context(), auth.UserFromContext(r.Context()).ID, data)
		if err != nil {
			writeStickerError(w, d, err)
			return
		}
		writeJSON(w, http.StatusCreated, stickerResponse{ID: added.ID.String(), Type: added.Type})
	}
}

func listStickersHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stickers, err := d.Sticker.List(r.Context(), auth.UserFromContext(r.Context()).ID)
		if err != nil {
			writeStickerError(w, d, err)
			return
		}

		writeJSON(w, http.StatusOK, stickerResponses(stickers))
	}
}

// setStickerPinsHandler replaces the caller's quick rail and answers with the
// library, so the picker that just saved does not need a second round trip to
// redraw itself.
func setStickerPinsHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setStickerPinsRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		stickerIDs := make([]uuid.UUID, 0, len(req.StickerIDs))
		for _, raw := range req.StickerIDs {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest,
					"sticker_ids must be UUIDs", "sticker_ids")
				return
			}
			stickerIDs = append(stickerIDs, parsed)
		}

		userID := auth.UserFromContext(r.Context()).ID
		if err := d.Sticker.SetPins(r.Context(), userID, stickerIDs); err != nil {
			writeStickerError(w, d, err)
			return
		}

		stickers, err := d.Sticker.List(r.Context(), userID)
		if err != nil {
			writeStickerError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, stickerResponses(stickers))
	}
}

func deleteStickerHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stickerID, ok := pathUUID(w, r, "sticker_id")
		if !ok {
			return
		}

		if err := d.Sticker.Remove(r.Context(),
			auth.UserFromContext(r.Context()).ID, stickerID); err != nil {
			writeStickerError(w, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getStickerMediaHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stickerID, ok := pathUUID(w, r, "sticker_id")
		if !ok {
			return
		}

		reader, contentType, err := d.Sticker.Open(r.Context(),
			auth.UserFromContext(r.Context()).ID, stickerID)
		if err != nil {
			writeStickerError(w, d, err)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", contentType)
		// 60 seconds, the same as an avatar, because it is the same rule: who
		// may see this follows current group membership, and the cache is the
		// one window in which a former member's browser can still draw it. The
		// bytes never change, so a longer life would be free — what is being
		// bounded is how long a stale permission survives, not how long the
		// image stays good.
		w.Header().Set("Cache-Control", "private, max-age=60")
		if _, err := io.Copy(w, reader); err != nil {
			d.Logger.Error("streaming sticker failed", "error", err)
		}
	}
}

func writeStickerError(w http.ResponseWriter, d Deps, err error) {
	var invalid sticker.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message, invalid.Field)
	case errors.Is(err, sticker.ErrNotFound):
		// One answer for "does not exist" and "not yours", the same as meals,
		// photos and chat media. Anything else confirms which sticker ids are
		// real to whoever is guessing.
		writeError(w, http.StatusNotFound, "not found")
	default:
		d.Logger.Error("sticker request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
