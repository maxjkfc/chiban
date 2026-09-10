package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// dailySummary is the response shape of GET /api/v1/meals/daily-summary.
type dailySummaryDay struct {
	Date      string   `json:"date"`
	MealCount int      `json:"meal_count"`
	MealTypes []string `json:"meal_types"`
}

type dailySummaryResponse struct {
	Days []dailySummaryDay `json:"days"`
}

func dailySummary(t *testing.T, app *testsupport.App, start, end string) *http.Response {
	t.Helper()
	return app.Request(http.MethodGet, "/api/v1/meals/daily-summary?start="+start+"&end="+end, nil)
}

// A day with no meals still has to appear, with an empty (not null) type
// list, so the frontend never has to synthesize the gap itself.
func TestDailySummaryFillsInEveryDayIncludingEmptyOnes(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := dailySummary(t, app, "2026-03-01", "2026-03-03")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var out dailySummaryResponse
	app.DecodeJSON(resp, &out)
	if len(out.Days) != 3 {
		t.Fatalf("days = %d, want 3", len(out.Days))
	}
	wantDates := []string{"2026-03-01", "2026-03-02", "2026-03-03"}
	for i, day := range out.Days {
		if day.Date != wantDates[i] {
			t.Fatalf("days[%d].date = %q, want %q", i, day.Date, wantDates[i])
		}
		if day.MealCount != 0 {
			t.Fatalf("days[%d].meal_count = %d, want 0", i, day.MealCount)
		}
		if day.MealTypes == nil || len(day.MealTypes) != 0 {
			t.Fatalf("days[%d].meal_types = %v, want an empty (non-null) slice", i, day.MealTypes)
		}
	}
}

// Meal counts must reflect the caller's own timezone, not the server's local
// clock or UTC: a supper eaten in the small hours UTC is already the next
// day in Taipei, and the summary has to bucket it there.
func TestDailySummaryUsesTheProfileTimezone(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	// 16:30 UTC on the 14th is 00:30 on the 15th in Taipei.
	supper := time.Date(2026, 3, 14, 16, 30, 0, 0, time.UTC)
	app.CreateMealAt(supper, testsupport.JPEG(t, 100, 100))

	resp := dailySummary(t, app, "2026-03-14", "2026-03-15")
	var out dailySummaryResponse
	app.DecodeJSON(resp, &out)
	if len(out.Days) != 2 {
		t.Fatalf("days = %d, want 2", len(out.Days))
	}
	if out.Days[0].MealCount != 0 {
		t.Fatalf("14th meal_count = %d, want 0: it was already the 15th in Taipei", out.Days[0].MealCount)
	}
	if out.Days[1].MealCount != 1 {
		t.Fatalf("15th meal_count = %d, want 1", out.Days[1].MealCount)
	}
}

// meal_count is not capped at three: recording the same meal type twice must
// still show up as two entries in meal_types, not be silently collapsed.
func TestDailySummaryCountsDuplicateMealsOnTheSameDay(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")

	// Four meals on the same local day: two lunches, a dinner and an
	// untyped snack.
	base := time.Date(2026, 3, 15, 3, 0, 0, 0, time.UTC) // 11:00 Taipei
	app.UploadMeal(map[string]string{
		"eaten_at": base.Format(time.RFC3339), "meal_type": "lunch",
	}, testsupport.JPEG(t, 100, 100))
	app.UploadMeal(map[string]string{
		"eaten_at": base.Add(1 * time.Hour).Format(time.RFC3339), "meal_type": "lunch",
	}, testsupport.JPEG(t, 100, 100))
	app.UploadMeal(map[string]string{
		"eaten_at": base.Add(2 * time.Hour).Format(time.RFC3339), "meal_type": "dinner",
	}, testsupport.JPEG(t, 100, 100))
	app.UploadMeal(map[string]string{
		"eaten_at": base.Add(3 * time.Hour).Format(time.RFC3339),
	}, testsupport.JPEG(t, 100, 100))

	resp := dailySummary(t, app, "2026-03-15", "2026-03-15")
	var out dailySummaryResponse
	app.DecodeJSON(resp, &out)
	if len(out.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(out.Days))
	}
	day := out.Days[0]
	if day.MealCount != 4 {
		t.Fatalf("meal_count = %d, want 4 (not capped at 3)", day.MealCount)
	}
	if len(day.MealTypes) != 4 {
		t.Fatalf("meal_types = %v, want 4 entries", day.MealTypes)
	}
	wantTypes := []string{"lunch", "lunch", "dinner", ""}
	for i, want := range wantTypes {
		if day.MealTypes[i] != want {
			t.Fatalf("meal_types[%d] = %q, want %q", i, day.MealTypes[i], want)
		}
	}
}

// Only the caller's own meals may ever be counted.
func TestDailySummaryOnlyCountsTheCallersOwnMeals(t *testing.T) {
	app := testsupport.NewApp(t)
	app.RegisterUser("mei@example.com")
	app.SaveProfile("小美", "Asia/Taipei")
	app.CreateMeal(testsupport.JPEG(t, 100, 100))
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")

	today := time.Now().In(mustTaipei(t)).Format("2006-01-02")
	resp := dailySummary(t, app, today, today)
	var out dailySummaryResponse
	app.DecodeJSON(resp, &out)
	if len(out.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(out.Days))
	}
	if out.Days[0].MealCount != 0 {
		t.Fatalf("meal_count = %d, want 0: another user's meal must not leak in", out.Days[0].MealCount)
	}
}

func TestDailySummaryRejectsARangeOverTheLimit(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	// 93 days, one more than the 92-day cap.
	resp := dailySummary(t, app, "2026-01-01", "2026-04-04")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestDailySummaryAcceptsExactlyTheRangeLimit(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	// 92 days inclusive, exactly at the cap.
	resp := dailySummary(t, app, "2026-01-01", "2026-04-02")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var out dailySummaryResponse
	app.DecodeJSON(resp, &out)
	if len(out.Days) != 92 {
		t.Fatalf("days = %d, want 92", len(out.Days))
	}
}

func TestDailySummaryRejectsEndBeforeStart(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := dailySummary(t, app, "2026-03-15", "2026-03-10")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestDailySummaryRejectsMissingOrMalformedDates(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	cases := []struct {
		name       string
		start, end string
	}{
		{"missing start", "", "2026-03-15"},
		{"missing end", "2026-03-01", ""},
		{"malformed start", "2026/03/01", "2026-03-15"},
		{"malformed end", "2026-03-01", "15-03-2026"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := dailySummary(t, app, tc.start, tc.end)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

func TestDailySummaryRequiresASession(t *testing.T) {
	app := testsupport.NewApp(t)

	resp := dailySummary(t, app, "2026-03-01", "2026-03-15")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func mustTaipei(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("load Asia/Taipei: %v", err)
	}
	return loc
}
