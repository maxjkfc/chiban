package meal

import (
	"fmt"
	"time"
)

// DateFormat is how a calendar date crosses the API boundary.
const DateFormat = "2006-01-02"

// Date is a calendar date with no time and no zone attached — the thing a user
// means by "the 5th", independent of where they were standing.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// ParseDate reads a YYYY-MM-DD date.
func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(DateFormat, value)
	if err != nil {
		return Date{}, InvalidInputError{Field: "date", Message: "must be a YYYY-MM-DD date"}
	}
	return Date{Year: parsed.Year(), Month: parsed.Month(), Day: parsed.Day()}, nil
}

// TodayIn is the calendar date it currently is for someone in loc. Server-local
// time is never the answer: two users eating at the same instant can be on
// different days.
func TodayIn(now time.Time, loc *time.Location) Date {
	local := now.In(loc)
	return Date{Year: local.Year(), Month: local.Month(), Day: local.Day()}
}

// DayRange is the half-open UTC interval [start, end) covering d in loc.
//
// This is the only place a calendar date turns into an instant range. Every
// Today and history query goes through it rather than doing its own
// AT TIME ZONE arithmetic, so there is one definition of where a user's day
// starts and one place to get daylight saving right.
//
// Days are not always 24 hours: a DST transition makes one 23 and another 25,
// and in some zones midnight itself does not exist. Deriving both ends from
// the same startOfDay handles all three, and guarantees consecutive days meet
// exactly once — no instant lands in two days or in none.
func DayRange(d Date, loc *time.Location) (start, end time.Time) {
	return startOfDay(d, loc).UTC(),
		startOfDay(Date{Year: d.Year, Month: d.Month, Day: d.Day + 1}, loc).UTC()
}

// startOfDay is the first instant that belongs to d in loc.
//
// Usually that is local midnight, but where the clock jumps forward at
// midnight — Chile does this — midnight never happens, and time.Date
// normalises it backwards into the previous day. Taking that as the start
// would put the previous evening's last hour into two days at once. Advancing
// to the first hour that really is on d gives the instant the day begins.
func startOfDay(d Date, loc *time.Location) time.Time {
	local := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
	// Normalise first, so a rolled-over day number (Day: 32) compares sanely.
	want := time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, loc)
	for hour := 1; hour <= 3 && local.Day() != want.Day(); hour++ {
		local = time.Date(d.Year, d.Month, d.Day, hour, 0, 0, 0, loc)
	}
	return local
}

// DateOf is the calendar date an instant falls on for someone in loc — the
// inverse of DayRange, used to label a meal with the day it belongs to.
func DateOf(instant time.Time, loc *time.Location) Date {
	local := instant.In(loc)
	return Date{Year: local.Year(), Month: local.Month(), Day: local.Day()}
}
