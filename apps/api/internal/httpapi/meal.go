package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/meal"
	"github.com/maxjkfc/chiban/apps/api/internal/media"
)

type mealResponse struct {
	ID       string `json:"id"`
	MealType string `json:"meal_type,omitempty"`
	// EatenAt is the instant; EatenAtLocal is the same moment as wall-clock
	// time in the owner's timezone. The frontend renders and edits the local
	// form and never does timezone arithmetic of its own — the browser's zone
	// is not necessarily the profile's, and guessing wrong would silently
	// rewrite the instant on save.
	EatenAt      string `json:"eaten_at"`
	EatenAtLocal string `json:"eaten_at_local"`
	Description  string `json:"description,omitempty"`
	// PhotoIDs are application-level IDs. Buckets and object names never leave
	// the backend, so the frontend cannot depend on the storage layout.
	PhotoIDs []string `json:"photo_ids"`
	// IsOwner tells the reader whether editing is theirs to do. Sharing means
	// people who cannot edit now read this same shape.
	IsOwner bool `json:"is_owner"`
}

func newMealResponse(m meal.Meal, loc *time.Location, readerID uuid.UUID) mealResponse {
	ids := make([]string, 0, len(m.Photos))
	for _, p := range m.Photos {
		ids = append(ids, p.ID.String())
	}
	return mealResponse{
		ID:           m.ID.String(),
		MealType:     m.MealType,
		EatenAt:      m.EatenAt.UTC().Format(timeFormat),
		EatenAtLocal: m.EatenAt.In(loc).Format(meal.LocalTimeFormat),
		Description:  m.Description,
		PhotoIDs:     ids,
		// Editing is the owner's alone. Saying so lets the client show a
		// shared reader the meal without an edit form that would only ever
		// fail; the server still refuses either way.
		IsOwner: m.UserID == readerID,
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

		userID := auth.UserFromContext(r.Context()).ID
		loc, ok := userLocation(w, r, d, userID)
		if !ok {
			return
		}

		m, err := d.Meal.Create(r.Context(), userID, meal.Input{
			MealType:    r.FormValue("meal_type"),
			EatenAt:     eatenAt,
			Description: r.FormValue("description"),
		}, uploads)
		if err != nil {
			writeMealError(w, d, err)
			return
		}
		writeJSON(w, http.StatusCreated, newMealResponse(m, loc, userID))
	}
}

func getMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}

		userID := auth.UserFromContext(r.Context()).ID
		m, err := d.Meal.Get(r.Context(), userID, mealID)
		if err != nil {
			writeMealError(w, d, err)
			return
		}

		// The owner's timezone, not the reader's. eaten_at_local is a fact
		// about the owner's day — a lunch eaten at 12:30 in Taipei reads 12:30
		// to everyone it is shared with, rather than shifting to each reader's
		// clock. Before sharing existed the two were always the same person,
		// which is what kept this hidden until now.
		loc, ok := userLocation(w, r, d, m.UserID)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, newMealResponse(m, loc, userID))
	}
}

type patchMealRequest struct {
	// Pointers so an omitted field stays untouched: editing a note must not
	// silently reset the time.
	MealType *string `json:"meal_type"`
	// Wall-clock time in the owner's timezone, e.g. "2026-03-15T12:30". The
	// server resolves it against the profile zone; the browser's own zone
	// never enters into it.
	EatenAtLocal *string `json:"eaten_at_local"`
	Description  *string `json:"description"`
}

type mealDayResponse struct {
	Date  string         `json:"date"`
	Meals []mealResponse `json:"meals"`
}

// listMealsHandler answers "what did I eat on this day", where the day is the
// user's own — the profile timezone decides it, not the server clock.
func listMealsHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserFromContext(r.Context()).ID

		loc, ok := userLocation(w, r, d, userID)
		if !ok {
			return
		}

		date := meal.TodayIn(time.Now(), loc)
		if raw := r.URL.Query().Get("date"); raw != "" {
			parsed, err := meal.ParseDate(raw)
			if err != nil {
				writeMealError(w, d, err)
				return
			}
			date = parsed
		}

		meals, err := d.Meal.ListForDay(r.Context(), userID, date, loc)
		if err != nil {
			writeMealError(w, d, err)
			return
		}

		out := make([]mealResponse, 0, len(meals))
		for _, m := range meals {
			out = append(out, newMealResponse(m, loc, userID))
		}
		writeJSON(w, http.StatusOK, mealDayResponse{Date: date.String(), Meals: out})
	}
}

func patchMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}

		var req patchMealRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		userID := auth.UserFromContext(r.Context()).ID
		loc, ok := userLocation(w, r, d, userID)
		if !ok {
			return
		}

		in := meal.UpdateInput{MealType: req.MealType, Description: req.Description}
		if req.EatenAtLocal != nil {
			eatenAt, err := time.ParseInLocation(meal.LocalTimeFormat, *req.EatenAtLocal, loc)
			if err != nil {
				writeError(w, http.StatusBadRequest,
					"eaten_at_local must look like 2026-03-15T12:30", "eaten_at_local")
				return
			}
			in.EatenAt = &eatenAt
		}

		m, err := d.Meal.Update(r.Context(), userID, mealID, in)
		if err != nil {
			writeMealError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newMealResponse(m, loc, userID))
	}
}

func deleteMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}

		if err := d.Meal.Delete(r.Context(), auth.UserFromContext(r.Context()).ID, mealID); err != nil {
			writeMealError(w, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// userLocation resolves the requester's timezone. The meal domain never reads
// profiles itself; the handler brings the zone to it.
func userLocation(w http.ResponseWriter, r *http.Request, d Deps, userID uuid.UUID) (*time.Location, bool) {
	p, err := d.Profile.Get(r.Context(), userID)
	if err != nil {
		writeProfileError(w, d, err)
		return nil, false
	}

	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		// The profile only ever accepts loadable zones, so this means the
		// stored value went bad rather than the user sending something wrong.
		d.Logger.Error("profile has an unloadable timezone", "timezone", p.Timezone, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	return loc, true
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
	case errors.Is(err, meal.ErrNotMember):
		writeError(w, http.StatusForbidden, "you are not a member of that group", "group_ids")
	case errors.Is(err, meal.ErrNotFound), errors.Is(err, meal.ErrPhotoNotFound):
		// One answer for "does not exist" and "not yours": otherwise the API
		// confirms which meal and photo IDs are real.
		writeError(w, http.StatusNotFound, "not found")
	default:
		d.Logger.Error("meal request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type shareMealRequest struct {
	GroupIDs []string `json:"group_ids"`
}

type mealSharesResponse struct {
	GroupIDs []string `json:"group_ids"`
}

func shareMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}

		var req shareMealRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		groupIDs := make([]uuid.UUID, 0, len(req.GroupIDs))
		for _, raw := range req.GroupIDs {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "group_ids must be UUIDs", "group_ids")
				return
			}
			groupIDs = append(groupIDs, parsed)
		}

		userID := auth.UserFromContext(r.Context()).ID
		if err := d.Meal.Share(r.Context(), userID, mealID, groupIDs); err != nil {
			writeMealError(w, d, err)
			return
		}
		writeMealShares(w, d, r, mealID, userID)
	}
}

func listMealSharesHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}
		writeMealShares(w, d, r, mealID, auth.UserFromContext(r.Context()).ID)
	}
}

func unshareMealHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mealID, ok := pathUUID(w, r, "meal_id")
		if !ok {
			return
		}
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}

		if err := d.Meal.Unshare(r.Context(),
			auth.UserFromContext(r.Context()).ID, mealID, groupID); err != nil {
			writeMealError(w, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeMealShares(w http.ResponseWriter, d Deps, r *http.Request, mealID, userID uuid.UUID) {
	groupIDs, err := d.Meal.SharedWith(r.Context(), userID, mealID)
	if err != nil {
		writeMealError(w, d, err)
		return
	}

	out := mealSharesResponse{GroupIDs: make([]string, 0, len(groupIDs))}
	for _, id := range groupIDs {
		out.GroupIDs = append(out.GroupIDs, id.String())
	}
	writeJSON(w, http.StatusOK, out)
}
