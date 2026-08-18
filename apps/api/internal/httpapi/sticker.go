package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/media"
	"github.com/maxjkfc/chiban/apps/api/internal/sticker"
)

type stickerResponse struct {
	ID string `json:"id"`
	// Type is "image" or "gif". The client needs it only to know that a GIF
	// will animate; both are rendered the same way.
	Type string `json:"type"`
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

		out := make([]stickerResponse, 0, len(stickers))
		for _, one := range stickers {
			out = append(out, stickerResponse{ID: one.ID.String(), Type: one.Type})
		}
		writeJSON(w, http.StatusOK, out)
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
		// A sticker's bytes never change and it is meant to be sent over and
		// over, so this is the one image in the product worth caching hard.
		// Permission can still change, which is what bounds it to an hour.
		w.Header().Set("Cache-Control", "private, max-age=3600")
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
	case errors.Is(err, sticker.ErrTooMany):
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("you can keep at most %d stickers", sticker.MaxPerUser), "file")
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
