package meal_test

import (
	"testing"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/meal"
)

// Day boundaries are one of the few things the spec allows a unit test for:
// pure input to output, with edge cases (midnight, DST, zones that skip
// midnight entirely) that are not worth enumerating through HTTP.

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

// The case that decides whether late-night eating lands on the right day: in
// Taipei, 16:30 UTC is already half past midnight tomorrow.
func TestLateNightMealBelongsToTheLocalDay(t *testing.T) {
	taipei := mustLoad(t, "Asia/Taipei")
	supper := time.Date(2026, 3, 14, 16, 30, 0, 0, time.UTC)

	got := meal.DateOf(supper, taipei)

	want := meal.Date{Year: 2026, Month: time.March, Day: 15}
	if got != want {
		t.Fatalf("date = %s, want %s", got, want)
	}
}

func TestDayRangeCoversExactlyTheLocalDay(t *testing.T) {
	taipei := mustLoad(t, "Asia/Taipei")
	date := meal.Date{Year: 2026, Month: time.March, Day: 15}

	start, end := meal.DayRange(date, taipei)

	// Taipei is UTC+8 all year, so the day runs 16:00 UTC to 16:00 UTC.
	wantStart := time.Date(2026, 3, 14, 16, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 3, 15, 16, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("range = [%s, %s), want [%s, %s)", start, end, wantStart, wantEnd)
	}
}

// The boundary is half-open: the instant a day ends belongs to the next day,
// and nothing may fall into both or neither.
func TestDayRangeBoundariesDoNotOverlap(t *testing.T) {
	taipei := mustLoad(t, "Asia/Taipei")

	_, endOfFirst := meal.DayRange(meal.Date{Year: 2026, Month: time.March, Day: 15}, taipei)
	startOfSecond, _ := meal.DayRange(meal.Date{Year: 2026, Month: time.March, Day: 16}, taipei)

	if !endOfFirst.Equal(startOfSecond) {
		t.Fatalf("day 15 ends at %s but day 16 starts at %s", endOfFirst, startOfSecond)
	}
	if got := meal.DateOf(endOfFirst, taipei); got.Day != 16 {
		t.Fatalf("the instant a day ends belongs to day %d, want 16", got.Day)
	}
}

// A day is not always 24 hours. Deriving both ends from successive local
// midnights is what makes this come out right without special cases.
func TestDaylightSavingChangesDayLength(t *testing.T) {
	newYork := mustLoad(t, "America/New_York")

	cases := []struct {
		name string
		date meal.Date
		want time.Duration
	}{
		{"spring forward", meal.Date{Year: 2026, Month: time.March, Day: 8}, 23 * time.Hour},
		{"autumn back", meal.Date{Year: 2026, Month: time.November, Day: 1}, 25 * time.Hour},
		{"ordinary day", meal.Date{Year: 2026, Month: time.June, Day: 1}, 24 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := meal.DayRange(tc.date, newYork)

			if got := end.Sub(start); got != tc.want {
				t.Fatalf("%s is %s long, want %s", tc.date, got, tc.want)
			}
		})
	}
}

// Some zones skip midnight altogether when DST starts. The day still has to
// have a start, and it still has to join up with the previous day.
func TestZoneThatSkipsMidnightStillHasAContinuousDay(t *testing.T) {
	santiago := mustLoad(t, "America/Santiago")
	// Chile moves the clock forward at midnight, so 2026-09-06 00:00 local
	// does not exist.
	date := meal.Date{Year: 2026, Month: time.September, Day: 6}

	start, end := meal.DayRange(date, santiago)
	_, previousEnd := meal.DayRange(meal.Date{Year: 2026, Month: time.September, Day: 5}, santiago)

	if !start.Equal(previousEnd) {
		t.Fatalf("day starts at %s but the previous day ended at %s", start, previousEnd)
	}
	if !end.After(start) {
		t.Fatalf("day range [%s, %s) is empty or reversed", start, end)
	}
	if got := meal.DateOf(start, santiago); got != date {
		t.Fatalf("the first instant of %s is labelled %s", date, got)
	}
}

// Two people eating at the same instant can be on different days.
func TestTheSameInstantIsADifferentDayInADifferentZone(t *testing.T) {
	instant := time.Date(2026, 3, 14, 23, 0, 0, 0, time.UTC)

	taipei := meal.DateOf(instant, mustLoad(t, "Asia/Taipei"))
	newYork := meal.DateOf(instant, mustLoad(t, "America/New_York"))

	if taipei.Day != 15 {
		t.Fatalf("Taipei date = %s, want the 15th", taipei)
	}
	if newYork.Day != 14 {
		t.Fatalf("New York date = %s, want the 14th", newYork)
	}
}

func TestParseDateRejectsAnythingElse(t *testing.T) {
	for _, value := range []string{"", "2026-3-15", "15/03/2026", "2026-13-01", "yesterday"} {
		if _, err := meal.ParseDate(value); err == nil {
			t.Fatalf("accepted %q as a date", value)
		}
	}
}

func TestTodayInUsesTheUsersZoneNotTheServers(t *testing.T) {
	// Just before midnight UTC, which is already tomorrow in Taipei.
	now := time.Date(2026, 3, 14, 23, 30, 0, 0, time.UTC)

	if got := meal.TodayIn(now, mustLoad(t, "Asia/Taipei")); got.Day != 15 {
		t.Fatalf("today in Taipei = %s, want the 15th", got)
	}
	if got := meal.TodayIn(now, time.UTC); got.Day != 14 {
		t.Fatalf("today in UTC = %s, want the 14th", got)
	}
}

// AddDays has to cross month and year boundaries the same way it crosses an
// ordinary day, since it is what walks a summary range one date at a time.
func TestDateAddDaysCrossesMonthAndYearBoundaries(t *testing.T) {
	cases := []struct {
		name string
		date meal.Date
		n    int
		want meal.Date
	}{
		{"ordinary", meal.Date{Year: 2026, Month: time.March, Day: 14}, 1, meal.Date{Year: 2026, Month: time.March, Day: 15}},
		{"month end", meal.Date{Year: 2026, Month: time.March, Day: 31}, 1, meal.Date{Year: 2026, Month: time.April, Day: 1}},
		{"year end", meal.Date{Year: 2025, Month: time.December, Day: 31}, 1, meal.Date{Year: 2026, Month: time.January, Day: 1}},
		{"negative", meal.Date{Year: 2026, Month: time.March, Day: 1}, -1, meal.Date{Year: 2026, Month: time.February, Day: 28}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.date.AddDays(tc.n); got != tc.want {
				t.Fatalf("%s.AddDays(%d) = %s, want %s", tc.date, tc.n, got, tc.want)
			}
		})
	}
}

func TestDateDaysUntilIsTheInverseOfAddDays(t *testing.T) {
	start := meal.Date{Year: 2026, Month: time.March, Day: 1}
	end := meal.Date{Year: 2026, Month: time.April, Day: 5}

	n := start.DaysUntil(end)
	if got := start.AddDays(n); got != end {
		t.Fatalf("start.AddDays(start.DaysUntil(end)) = %s, want %s", got, end)
	}
	if n <= 0 {
		t.Fatalf("DaysUntil(end) = %d, want a positive count since end is later", n)
	}

	if got := end.DaysUntil(start); got != -n {
		t.Fatalf("end.DaysUntil(start) = %d, want %d", got, -n)
	}
}
