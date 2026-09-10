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

function dateAtUTC(date: string): Date {
  return new Date(`${date}T12:00:00Z`);
}

function dateString(date: Date): string {
  return date.toISOString().slice(0, 10);
}

/** Caps a day's count at the three trophy slots shown in the UI. */
export function trophyProgress(mealCount: number): 0 | 1 | 2 | 3 {
  if (mealCount >= 3) return 3;
  if (mealCount <= 0) return 0;
  return mealCount as 1 | 2;
}

/** Returns the inclusive seven-day range centered on the user's local today. */
export function weekRange(today: string): DateRange {
  const start = dateAtUTC(today);
  start.setUTCDate(start.getUTCDate() - 3);
  const end = dateAtUTC(today);
  end.setUTCDate(end.getUTCDate() + 3);
  return { start: dateString(start), end: dateString(end) };
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
