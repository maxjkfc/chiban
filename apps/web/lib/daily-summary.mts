export type DailySummaryDay = {
  date: string;
  meal_count: number;
  meal_types: string[];
};

export type DailySummaryResponse = {
  days: DailySummaryDay[];
};

export type DateRange = {
  start: string;
  end: string;
};

/** Keep client navigation aligned with the API's inclusive range limit. */
export const MAX_DAILY_SUMMARY_RANGE_DAYS = 92;
export const TROPHY_WINDOW_RADIUS_DAYS = 3;

function dateAtUTC(date: string): Date {
  return new Date(`${date}T12:00:00Z`);
}

function dateString(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function shiftCalendarDate(date: string, amount: number): string {
  const shifted = dateAtUTC(date);
  shifted.setUTCDate(shifted.getUTCDate() + amount);
  return dateString(shifted);
}

/** Caps a day's count at the three trophy slots shown in the UI. */
export function trophyProgress(mealCount: number): 0 | 1 | 2 | 3 {
  if (mealCount >= 3) return 3;
  if (mealCount <= 0) return 0;
  return mealCount as 1 | 2;
}

/** Returns the inclusive seven-day range centered on the user's local today. */
export function weekRange(today: string): DateRange {
  const start = shiftCalendarDate(today, -TROPHY_WINDOW_RADIUS_DAYS);
  const end = shiftCalendarDate(today, TROPHY_WINDOW_RADIUS_DAYS);
  return { start, end };
}

/**
 * Returns the dates that can be used as the trophy anchor. The boundary keeps
 * the rail from navigating beyond the same 92-day history envelope used by
 * daily-summary. The seven-day request itself remains well below the API cap.
 */
export function trophyAnchorBounds(today: string): DateRange {
  return {
    start: shiftCalendarDate(today, -MAX_DAILY_SUMMARY_RANGE_DAYS),
    end: shiftCalendarDate(today, MAX_DAILY_SUMMARY_RANGE_DAYS),
  };
}

export function isTrophyAnchorInBounds(anchor: string, today: string): boolean {
  const bounds = trophyAnchorBounds(today);
  return anchor >= bounds.start && anchor <= bounds.end;
}

export function canShiftTrophyAnchor(
  anchor: string,
  amount: number,
  today: string,
): boolean {
  return isTrophyAnchorInBounds(shiftCalendarDate(anchor, amount), today);
}

export function isDailySummaryRangeWithinLimit(range: DateRange): boolean {
  const start = dateAtUTC(range.start);
  const end = dateAtUTC(range.end);
  return end >= start &&
    Math.floor((end.getTime() - start.getTime()) / 86_400_000) + 1 <=
      MAX_DAILY_SUMMARY_RANGE_DAYS;
}

/** Expands an inclusive date range for rendering a fixed row or grid. */
export function dateRangeDates(range: DateRange): string[] {
  const dates: string[] = [];
  const current = dateAtUTC(range.start);
  const end = dateAtUTC(range.end);
  while (current <= end) {
    dates.push(dateString(current));
    current.setUTCDate(current.getUTCDate() + 1);
  }
  return dates;
}

/** Returns the inclusive first and last dates for a YYYY-MM calendar month. */
export function monthRange(month: string): DateRange {
  const [year, monthNumber] = month.split("-").map(Number);
  const last = new Date(Date.UTC(year, monthNumber, 0, 12));
  return {
    start: `${month}-01`,
    end: dateString(last),
  };
}

/** Moves a YYYY-MM value by whole calendar months. */
export function shiftMonth(month: string, amount: number): string {
  const [year, monthNumber] = month.split("-").map(Number);
  const shifted = new Date(Date.UTC(year, monthNumber - 1 + amount, 1, 12));
  return shifted.toISOString().slice(0, 7);
}

/**
 * Builds a Monday-first grid. Null cells are the leading/trailing days outside
 * the requested month, so the calendar can render a stable seven-column grid.
 */
export function calendarCells(month: string): Array<string | null> {
  const [year, monthNumber] = month.split("-").map(Number);
  const first = new Date(Date.UTC(year, monthNumber - 1, 1, 12));
  const daysInMonth = new Date(Date.UTC(year, monthNumber, 0, 12)).getUTCDate();
  const leading = (first.getUTCDay() + 6) % 7;
  const cells: Array<string | null> = Array.from({ length: leading }, () => null);

  for (let day = 1; day <= daysInMonth; day += 1) {
    cells.push(`${month}-${String(day).padStart(2, "0")}`);
  }

  while (cells.length % 7 !== 0) cells.push(null);
  return cells;
}
