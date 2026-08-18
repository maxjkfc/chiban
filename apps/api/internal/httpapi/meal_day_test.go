package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// Which day a meal belongs to is decided by the eater's timezone. This is the
// end-to-end version of that: the pure function is unit tested separately, but
// it only matters if the query actually uses the profile's zone.
func TestTodayFollowsTheProfileTimezone(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	// 16:30 UTC is 00:30 the next day in Taipei.
	supper := time.Date(2026, 3, 14, 16, 30, 0, 0, time.UTC)
	app.CreateMealAt(supper, testsupport.JPEG(t, 100, 100))

	onTheFourteenth := app.MealsOn("2026-03-14")
	if len(onTheFourteenth) != 0 {
		t.Fatalf("14th has %d meals, want 0: it was already the 15th in Taipei", len(onTheFourteenth))
	}

	onTheFifteenth := app.MealsOn("2026-03-15")
	if len(onTheFifteenth) != 1 {
		t.Fatalf("15th has %d meals, want 1", len(onTheFifteenth))
	}
}

// Moving timezone moves the day a meal belongs to. Nothing about the meal
// changes; the question "which day was that" simply has a different answer.
func TestChangingTimezoneMovesTheMealToAnotherDay(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	supper := time.Date(2026, 3, 14, 16, 30, 0, 0, time.UTC)
	app.CreateMealAt(supper, testsupport.JPEG(t, 100, 100))

	if got := len(app.MealsOn("2026-03-15")); got != 1 {
		t.Fatalf("in Taipei the 15th has %d meals, want 1", got)
	}

	app.SaveProfile("小美", "America/New_York")

	if got := len(app.MealsOn("2026-03-15")); got != 0 {
		t.Fatalf("in New York the 15th has %d meals, want 0", got)
	}
	if got := len(app.MealsOn("2026-03-14")); got != 1 {
		t.Fatalf("in New York the 14th has %d meals, want 1", got)
	}
}

// The boundary itself: the last instant of a day and the first of the next
// must land on different days, with nothing lost between them.
func TestMealsExactlyOnTheDayBoundary(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	photo := testsupport.JPEG(t, 100, 100)
	// Taipei is UTC+8: the 15th runs from 2026-03-14 16:00 UTC.
	app.CreateMealAt(time.Date(2026, 3, 14, 15, 59, 59, 0, time.UTC), photo)
	app.CreateMealAt(time.Date(2026, 3, 14, 16, 0, 0, 0, time.UTC), photo)

	if got := len(app.MealsOn("2026-03-14")); got != 1 {
		t.Fatalf("14th has %d meals, want 1", got)
	}
	if got := len(app.MealsOn("2026-03-15")); got != 1 {
		t.Fatalf("15th has %d meals, want 1", got)
	}
}

func TestListingWithoutADateReturnsToday(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	app.CreateMeal(testsupport.JPEG(t, 100, 100))

	resp := app.Request(http.MethodGet, "/api/v1/meals", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var day struct {
		Date  string             `json:"date"`
		Meals []testsupport.Meal `json:"meals"`
	}
	app.DecodeJSON(resp, &day)

	taipei, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	if want := time.Now().In(taipei).Format("2006-01-02"); day.Date != want {
		t.Fatalf("date = %q, want today in Taipei (%q)", day.Date, want)
	}
	if len(day.Meals) != 1 {
		t.Fatalf("today has %d meals, want 1", len(day.Meals))
	}
}

func TestListingRejectsAMalformedDate(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := app.Request(http.MethodGet, "/api/v1/meals?date=15%2F03%2F2026", nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestEditingAMeal(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 100, 100))

	resp := app.Request(http.MethodPatch, "/api/v1/meals/"+created.ID, map[string]string{
		"meal_type":   "dinner",
		"description": "改成晚餐",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var updated testsupport.Meal
	app.DecodeJSON(resp, &updated)
	if updated.MealType != "dinner" || updated.Description != "改成晚餐" {
		t.Fatalf("updated = %+v, want dinner / 改成晚餐", updated)
	}
	if len(updated.PhotoIDs) != 1 {
		t.Fatalf("editing lost the photos: %v", updated.PhotoIDs)
	}
}

// A PATCH that omits a field must leave it alone, or editing a note would
// silently reset the time it was eaten.
func TestPatchLeavesOmittedMealFieldsAlone(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	eatenAt := time.Date(2026, 3, 15, 4, 0, 0, 0, time.UTC)
	created := app.CreateMealAt(eatenAt, testsupport.JPEG(t, 100, 100))
	app.Request(http.MethodPatch, "/api/v1/meals/"+created.ID, map[string]string{
		"meal_type": "lunch",
	})

	resp := app.Request(http.MethodPatch, "/api/v1/meals/"+created.ID, map[string]string{
		"description": "只改備註",
	})

	var updated testsupport.Meal
	app.DecodeJSON(resp, &updated)
	if updated.MealType != "lunch" {
		t.Fatalf("meal type = %q, want it left at lunch", updated.MealType)
	}
	if got, _ := time.Parse(time.RFC3339, updated.EatenAt); !got.Equal(eatenAt) {
		t.Fatalf("eaten_at = %s, want it left at %s", got, eatenAt)
	}
}

// The edit form sends wall-clock time. Interpreting it in the profile zone is
// the server's job: a browser in another zone must not shift the instant.
func TestEditingTheTimeUsesTheProfileZoneNotTheClients(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")
	created := app.CreateMealAt(time.Date(2026, 3, 15, 4, 0, 0, 0, time.UTC), testsupport.JPEG(t, 100, 100))

	resp := app.Request(http.MethodPatch, "/api/v1/meals/"+created.ID, map[string]string{
		"eaten_at_local": "2026-03-15T12:30",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var updated testsupport.Meal
	app.DecodeJSON(resp, &updated)

	// 12:30 in Taipei is 04:30 UTC, whatever zone the caller happens to be in.
	want := time.Date(2026, 3, 15, 4, 30, 0, 0, time.UTC)
	if got, _ := time.Parse(time.RFC3339, updated.EatenAt); !got.Equal(want) {
		t.Fatalf("eaten_at = %s, want %s", got, want)
	}
	if updated.EatenAtLocal != "2026-03-15T12:30" {
		t.Fatalf("eaten_at_local = %q, want it echoed back unchanged", updated.EatenAtLocal)
	}
}

// The same instant reads back as a different wall-clock time once the profile
// moves, which is what makes the edit form show the right thing.
func TestLocalTimeFollowsTheProfileZone(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")
	created := app.CreateMealAt(time.Date(2026, 3, 15, 4, 0, 0, 0, time.UTC), testsupport.JPEG(t, 100, 100))

	resp := app.Request(http.MethodGet, "/api/v1/meals/"+created.ID, nil)
	var inTaipei testsupport.Meal
	app.DecodeJSON(resp, &inTaipei)
	if inTaipei.EatenAtLocal != "2026-03-15T12:00" {
		t.Fatalf("in Taipei local = %q, want 2026-03-15T12:00", inTaipei.EatenAtLocal)
	}

	app.SaveProfile("小美", "America/New_York")

	resp = app.Request(http.MethodGet, "/api/v1/meals/"+created.ID, nil)
	var inNewYork testsupport.Meal
	app.DecodeJSON(resp, &inNewYork)
	if inNewYork.EatenAtLocal != "2026-03-15T00:00" {
		t.Fatalf("in New York local = %q, want 2026-03-15T00:00", inNewYork.EatenAtLocal)
	}
}

func TestEditingRejectsAMalformedLocalTime(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 100, 100))

	resp := app.Request(http.MethodPatch, "/api/v1/meals/"+created.ID, map[string]string{
		"eaten_at_local": "2026-03-15 12:30",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOnlyTheOwnerCanEditOrDeleteAMeal(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 100, 100))
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")

	patch := app.Request(http.MethodPatch, "/api/v1/meals/"+created.ID, map[string]string{
		"description": "不是我的",
	})
	if patch.StatusCode != http.StatusNotFound {
		t.Fatalf("patch status = %d, want %d", patch.StatusCode, http.StatusNotFound)
	}

	del := app.Request(http.MethodDelete, "/api/v1/meals/"+created.ID, nil)
	if del.StatusCode != http.StatusNotFound {
		t.Fatalf("delete status = %d, want %d", del.StatusCode, http.StatusNotFound)
	}
}

// Deleting is soft: the meal leaves the owner's history, but the row stays so
// the chat thread around it survives.
func TestDeletedMealsLeaveHistoryButKeepTheirRow(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	eatenAt := time.Date(2026, 3, 15, 4, 0, 0, 0, time.UTC)
	created := app.CreateMealAt(eatenAt, testsupport.JPEG(t, 100, 100))

	if resp := app.Request(http.MethodDelete, "/api/v1/meals/"+created.ID, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if got := len(app.MealsOn("2026-03-15")); got != 0 {
		t.Fatalf("history still shows %d meals after deletion", got)
	}
	if resp := app.Request(http.MethodGet, "/api/v1/meals/"+created.ID, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("reading a deleted meal: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var rows int
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM meal_records WHERE id = $1 AND deleted_at IS NOT NULL`, created.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatal("the meal row was removed; a deleted meal must leave a tombstone behind")
	}
}

func TestDeletingTwiceIsNotAnError(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	created := app.CreateMeal(testsupport.JPEG(t, 100, 100))

	app.Request(http.MethodDelete, "/api/v1/meals/"+created.ID, nil)
	second := app.Request(http.MethodDelete, "/api/v1/meals/"+created.ID, nil)

	// Already gone reads the same as never yours: one answer, no probing.
	if second.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", second.StatusCode, http.StatusNotFound)
	}
}
