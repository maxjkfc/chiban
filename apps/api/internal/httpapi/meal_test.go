package httpapi_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The shortest useful path through the product: a photo becomes a meal.
func TestCreatingAMealWithPhotos(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	created := app.CreateMeal(
		testsupport.JPEG(t, 400, 300),
		testsupport.JPEG(t, 400, 300),
	)

	if len(created.PhotoIDs) != 2 {
		t.Fatalf("photo ids = %v, want 2", created.PhotoIDs)
	}

	resp := app.Request(http.MethodGet, "/api/v1/meals/"+created.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var fetched testsupport.Meal
	app.DecodeJSON(resp, &fetched)
	if len(fetched.PhotoIDs) != 2 {
		t.Fatalf("fetched photo ids = %v, want 2", fetched.PhotoIDs)
	}
}

func TestMealPhotoCountLimits(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	photo := testsupport.JPEG(t, 100, 100)

	none := app.UploadMeal(nil)
	if none.StatusCode != http.StatusBadRequest {
		t.Fatalf("no photos: status = %d, want %d", none.StatusCode, http.StatusBadRequest)
	}

	tooMany := app.UploadMeal(nil, photo, photo, photo, photo, photo)
	if tooMany.StatusCode != http.StatusBadRequest {
		t.Fatalf("five photos: status = %d, want %d", tooMany.StatusCode, http.StatusBadRequest)
	}

	four := app.UploadMeal(nil, photo, photo, photo, photo)
	if four.StatusCode != http.StatusCreated {
		t.Fatalf("four photos: status = %d, want %d", four.StatusCode, http.StatusCreated)
	}
}

// A meal is private to its owner in this slice; sharing widens it later.
func TestMealAndItsPhotosAreOwnerOnly(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 200, 200))
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")

	meal := app.Request(http.MethodGet, "/api/v1/meals/"+created.ID, nil)
	if meal.StatusCode != http.StatusNotFound {
		t.Fatalf("meal status = %d, want %d", meal.StatusCode, http.StatusNotFound)
	}

	image := app.Request(http.MethodGet, "/api/v1/meal-images/"+created.PhotoIDs[0], nil)
	if image.StatusCode != http.StatusNotFound {
		t.Fatalf("image status = %d, want %d", image.StatusCode, http.StatusNotFound)
	}
}

func TestMealPhotoStreamsToItsOwner(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 200, 150))

	resp := app.Request(http.MethodGet, "/api/v1/meal-images/"+created.PhotoIDs[0], nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("streamed an empty image")
	}
}

// Every rejected photo reaches the user as the same failed upload, and the
// phones this product runs on cannot be reproduced locally: a HEIC, a ProRAW
// DNG and a photo iCloud never finished downloading are three different bugs
// wearing one message. The server's record of what the client declared is what
// tells them apart, so it is part of the contract rather than a convenience.
func TestARejectedPhotoIsLoggedWithWhatTheClientDeclared(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := app.UploadMeal(nil, []byte("this is not a photo"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	logs := app.Logs()
	for _, want := range []string{"upload not stored", "photo-0.jpg", "bytes=19"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log is missing %q, which is what makes the failure diagnosable:\n%s", want, logs)
		}
	}
}

func TestMealsRequireASession(t *testing.T) {
	app := testsupport.NewApp(t)

	create := app.UploadMeal(nil, testsupport.JPEG(t, 100, 100))
	if create.StatusCode != http.StatusUnauthorized {
		t.Fatalf("create status = %d, want %d", create.StatusCode, http.StatusUnauthorized)
	}
}

// The client addresses photos by ID only. There is no endpoint that takes a
// path, so a traversal attempt cannot even be expressed.
func TestPhotosAreNotAddressableByStoragePath(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 100, 100))

	var objectName string
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT object_name FROM meal_photos LIMIT 1`).Scan(&objectName); err != nil {
		t.Fatalf("read object name: %v", err)
	}

	for _, path := range []string{
		"/api/v1/meal-images/" + objectName,
		"/api/v1/meal-images/../../etc/passwd",
		"/api/v1/meal-images/meal-images%2F" + created.PhotoIDs[0],
	} {
		resp := app.Request(http.MethodGet, path, nil)
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("GET %s returned 200; storage paths must not be addressable", path)
		}
	}

	// And the response for a real meal never mentions where the bytes live.
	resp := app.Request(http.MethodGet, "/api/v1/meals/"+created.ID, nil)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, leaked := range []string{"meal-images", "object_name", "bucket", "users/"} {
		if strings.Contains(string(body), leaked) {
			t.Fatalf("meal response leaks storage detail %q: %s", leaked, body)
		}
	}
}

func TestRejectsFilesThatAreNotImages(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := app.UploadMeal(nil, []byte("this is definitely not a photo"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// Object storage and PostgreSQL are separate systems. When the second photo
// fails to upload, the first must not be left behind and no half-made meal may
// become visible.
func TestFailedUploadLeavesNoMealAndNoOrphanObject(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	app.Storage.FailUpload = errors.New("storage is down")
	app.Storage.FailUploadAfter = 1

	photo := testsupport.JPEG(t, 200, 200)
	resp := app.UploadMeal(nil, photo, photo, photo)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	if stored := app.Storage.Len(); stored != 0 {
		t.Fatalf("%d objects left in storage, want 0", stored)
	}

	var meals, photos int
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM meal_records`).Scan(&meals); err != nil {
		t.Fatalf("count meals: %v", err)
	}
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM meal_photos`).Scan(&photos); err != nil {
		t.Fatalf("count photos: %v", err)
	}
	if meals != 0 || photos != 0 {
		t.Fatalf("database has %d meals and %d photos, want none", meals, photos)
	}
}

func TestMealTypeAndDescriptionAreOptionalButValidated(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	photo := testsupport.JPEG(t, 100, 100)

	bare := app.UploadMeal(nil, photo)
	if bare.StatusCode != http.StatusCreated {
		t.Fatalf("meal without type or note: status = %d, want %d", bare.StatusCode, http.StatusCreated)
	}

	full := app.UploadMeal(map[string]string{
		"meal_type":   "lunch",
		"description": "今天外食",
		"eaten_at":    time.Now().UTC().Format(time.RFC3339),
	}, photo)
	if full.StatusCode != http.StatusCreated {
		t.Fatalf("full meal: status = %d, want %d", full.StatusCode, http.StatusCreated)
	}
	var created testsupport.Meal
	app.DecodeJSON(full, &created)
	if created.MealType != "lunch" || created.Description != "今天外食" {
		t.Fatalf("created = %+v, want lunch / 今天外食", created)
	}

	bogus := app.UploadMeal(map[string]string{"meal_type": "brunch"}, photo)
	if bogus.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid meal type: status = %d, want %d", bogus.StatusCode, http.StatusBadRequest)
	}
}

// eaten_at defaults to now so the fastest path is photo then publish.
func TestEatenAtDefaultsToNow(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	before := time.Now().UTC().Add(-time.Minute)
	created := app.CreateMeal(testsupport.JPEG(t, 100, 100))

	eatenAt, err := time.Parse(time.RFC3339, created.EatenAt)
	if err != nil {
		t.Fatalf("parse eaten_at %q: %v", created.EatenAt, err)
	}
	if eatenAt.Before(before) || eatenAt.After(time.Now().UTC().Add(time.Minute)) {
		t.Fatalf("eaten_at = %s, want roughly now", eatenAt)
	}
}
