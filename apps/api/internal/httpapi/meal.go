package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/meal"
	"github.com/maxjkfc/chiban/apps/api/internal/media"
)

type mealResponse struct {
	ID          string `json:"id"`
	MealType    string `json:"meal_type,omitempty"`
	EatenAt     string `json:"eaten_at"`
	Description string `json:"description,omitempty"`
	// PhotoIDs are application-level IDs. Buckets and object names never leave
	// the backend, so the frontend cannot depend on the storage layout.
	PhotoIDs []string `json:"photo_ids"`
}

func newMealResponse(m meal.Meal) mealResponse {
	ids := make([]string, 0, len(m.Photos))
	for _, p := range m.Photos {
		ids = append(ids, p.ID.String())
	}
	return mealResponse{
		ID:          m.ID.String(),
		MealType:    m.MealType,
		EatenAt:     m.EatenAt.UTC().Format(timeFormat),
		Description: m.Description,
		PhotoIDs:    ids,
	}
}

// maxRequestBytes caps a whole multipart upload: four photos at the media
// package's per-file limit, plus room for the form fields.
const maxRequestBytes = 4*media.MaxUploadBytes + (1 << 20)

// createMealHandler takes the meal and its photos in one request, which is
// what makes "all photos stored or no meal at all" possible; a two-step create
// would leave a meal visible with photos still missing.
func createMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeError(w, http.StatusBadRequest,
				"request must be multipart/form-data within the size limit")
			return
		}
		defer r.MultipartForm.RemoveAll()

		eatenAt, err := parseEatenAt(r.FormValue("eaten_at"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "eaten_at must be an RFC 3339 timestamp", "eaten_at")
			return
		}

		uploads, ok := readUploads(w, r)
		if !ok {
			return
		}

		m, err := d.Meal.Create(r.Context(), auth.UserFromContext(r.Context()).ID, meal.Input{
			MealType:    r.FormValue("meal_type"),
			EatenAt:     eatenAt,
			Description: r.FormValue("description"),
		}, uploads)
		if err != nil {
			writeMealError(w, d, err)
			return
		}
		writeJSON(w, http.StatusCreated, newMealResponse(m))
	}
}

func getMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}

		m, err := d.Meal.Get(r.Context(), auth.UserFromContext(r.Context()).ID, mealID)
		if err != nil {
			writeMealError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newMealResponse(m))
	}
}

// getMealImageHandler streams a photo through the API.
//
// The browser only ever supplies an image ID; the backend resolves it to a
// storage object itself. There is deliberately no endpoint that accepts a
// path, so no request can ask for an arbitrary object.
func getMealImageHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		imageID, ok := pathUUID(w, r, "image_id")
		if !ok {
			return
		}

		reader, contentType, err := d.Meal.OpenPhoto(r.Context(),
			auth.UserFromContext(r.Context()).ID, imageID)
		if err != nil {
			writeMealError(w, d, err)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", contentType)
		// Photos never change once stored, but they are private, so caching is
		// allowed only in the requesting user's own browser.
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if _, err := io.Copy(w, reader); err != nil {
			d.Logger.Error("streaming meal image failed", "error", err)
		}
	}
}

// readUploads collects the photo parts, rejecting a request that carries more
// than the maximum before any of it is read into memory.
func readUploads(w http.ResponseWriter, r *http.Request) ([]meal.Upload, bool) {
	files := r.MultipartForm.File["photos"]
	if len(files) > meal.MaxPhotos {
		writeError(w, http.StatusBadRequest,
			"at most "+strconv.Itoa(meal.MaxPhotos)+" photos are allowed", "photos")
		return nil, false
	}

	uploads := make([]meal.Upload, 0, len(files))
	for _, header := range files {
		if header.Size > media.MaxUploadBytes {
			writeError(w, http.StatusBadRequest,
				"each photo must be smaller than "+strconv.Itoa(media.MaxUploadBytes>>20)+" MB", "photos")
			return nil, false
		}

		file, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the uploaded photo", "photos")
			return nil, false
		}
		data, err := io.ReadAll(io.LimitReader(file, media.MaxUploadBytes+1))
		file.Close()
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the uploaded photo", "photos")
			return nil, false
		}
		uploads = append(uploads, meal.Upload{Data: data})
	}
	return uploads, true
}

// parseEatenAt defaults to now, so the fastest recording path does not have to
// send a timestamp at all.
func parseEatenAt(value string) (time.Time, error) {
	if value == "" {
		return time.Now().UTC(), nil
	}
	return time.Parse(time.RFC3339, value)
}

func writeMealError(w http.ResponseWriter, d Deps, err error) {
	var invalid meal.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message, invalid.Field)
	case errors.Is(err, meal.ErrNotFound), errors.Is(err, meal.ErrPhotoNotFound):
		// One answer for "does not exist" and "not yours": otherwise the API
		// confirms which meal and photo IDs are real.
		writeError(w, http.StatusNotFound, "not found")
	default:
		d.Logger.Error("meal request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
